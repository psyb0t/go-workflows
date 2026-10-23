package testinfra

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/psyb0t/ctxerrors"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type Kind string

const (
	MySQL          Kind = "mysql"
	Postgres       Kind = "postgres"
	Redis          Kind = "redis"
	startupTimeout      = 2 * time.Minute
	stopTimeout         = 15 * time.Second
	mysqlImage          = "mysql:8.4@sha256:76f0358884cb92d9a086d24e5671c38048d3fef40f2588703d73eb55392d6b69"
	postgresImage       = "postgres:16-alpine@sha256:f4610795587492cd4fb887c2dc3de301b813278bb6f616992061141b5f15ac36"
	redisImage          = "redis:6.2-alpine@sha256:592b215e9287182f11ba9d7c229dc0ba312bb900a016ea65ae378b0127567319"
)

type Infra struct {
	Host      string
	Port      int
	User      string
	Password  string
	container testcontainers.Container
	once      sync.Once
	closeErr  error
}

func (i *Infra) Address() string {
	return net.JoinHostPort(i.Host, strconv.Itoa(i.Port))
}

func (i *Infra) MySQLDSN(database string) string {
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&interpolateParams=true", i.User, i.Password, i.Address(), database)
}

func (i *Infra) PostgresDSN(database string) string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", i.Host, i.Port, i.User, i.Password, database)
}

func Start(ctx context.Context, kind Kind) (*Infra, error) {
	i := &Infra{User: "root", Password: uuid.NewString()}
	request := testcontainers.ContainerRequest{Name: "go-workflows-tests-" + string(kind) + "-" + uuid.NewString()}
	var port string
	switch kind {
	case MySQL:
		port = "3306/tcp"
		request.Image = mysqlImage
		request.Env = map[string]string{"MYSQL_ROOT_PASSWORD": i.Password, "MYSQL_ROOT_HOST": "%"}
		request.WaitingFor = wait.ForSQL(port, "mysql", func(host string, mapped network.Port) string {
			return fmt.Sprintf("root:%s@tcp(%s)/mysql", i.Password, net.JoinHostPort(host, mapped.Port()))
		}).WithStartupTimeout(startupTimeout)
	case Postgres:
		port = "5432/tcp"
		request.Image = postgresImage
		request.Env = map[string]string{"POSTGRES_USER": i.User, "POSTGRES_PASSWORD": i.Password}
		request.WaitingFor = wait.ForSQL(port, "pgx", func(host string, mapped network.Port) string {
			return fmt.Sprintf("host=%s port=%s user=root password=%s dbname=postgres sslmode=disable", host, mapped.Port(), i.Password)
		}).WithStartupTimeout(startupTimeout)
	case Redis:
		port = "6379/tcp"
		i.User = ""
		request.Image = redisImage
		request.Cmd = []string{"redis-server", "--requirepass", i.Password}
		request.WaitingFor = wait.ForListeningPort(port).WithStartupTimeout(startupTimeout)
	default:
		return nil, ctxerrors.Wrapf(errors.ErrUnsupported, "test backend %q", kind)
	}
	request.ExposedPorts = []string{port}
	request.HostConfigModifier = func(config *container.HostConfig) {
		config.PortBindings = network.PortMap{
			network.MustParsePort(port): {{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: "0"}},
		}
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: request, Started: true})
	i.container = container
	if err != nil {
		return nil, ctxerrors.Wrap(errors.Join(err, i.Close()), "start test backend")
	}
	if err := i.resolve(ctx, port); err != nil {
		return nil, ctxerrors.Wrap(errors.Join(err, i.Close()), "resolve test backend")
	}
	if kind == Redis {
		client := redis.NewClient(&redis.Options{Addr: i.Address(), Password: i.Password})
		pingErr := client.Ping(ctx).Err()
		closeErr := client.Close()
		if err := errors.Join(pingErr, closeErr); err != nil {
			return nil, ctxerrors.Wrap(errors.Join(err, i.Close()), "verify Redis readiness")
		}
	}
	return i, nil
}

func (i *Infra) resolve(ctx context.Context, port string) error {
	host, err := i.container.Host(ctx)
	if err != nil {
		return ctxerrors.Wrap(err, "resolve Docker host")
	}
	mapped, err := i.container.MappedPort(ctx, port)
	if err != nil {
		return ctxerrors.Wrap(err, "resolve backend port")
	}
	i.Host, i.Port = host, int(mapped.Num())
	return nil
}

func (i *Infra) Close() error {
	i.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
		defer cancel()
		i.closeErr = testcontainers.TerminateContainer(i.container, testcontainers.StopContext(ctx))
	})
	if i.closeErr != nil {
		return ctxerrors.Wrap(i.closeErr, "terminate owned test backend")
	}
	return nil
}

func Run(m *testing.M, kind Kind, destination **Infra) int {
	flag.Parse()
	if testing.Short() {
		return m.Run()
	}
	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()
	i, err := Start(ctx, kind)
	if err != nil {
		slog.ErrorContext(ctx, "test backend setup failed", "backend", kind, "err", err)
		return 1
	}
	*destination = i
	status := m.Run()
	if err := i.Close(); err != nil {
		slog.Error("test backend teardown failed", "backend", kind, "err", err)
		return 1
	}
	return status
}

// Exit runs cleanup before the test process exits.
func Exit(m *testing.M, kind Kind, destination **Infra) {
	os.Exit(Run(m, kind, destination))
}
