package redis

import (
	"context"
	"embed"
	"io/fs"
	"time"

	"github.com/psyb0t/ctxerrors"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/backend/metrics"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/internal/metrickeys"
)

var _ backend.Backend = (*redisBackend)(nil)

//go:embed scripts
var luaScripts embed.FS

var (
	createWorkflowInstanceCmd *redis.Script
	completeWorkflowTaskCmd   *redis.Script
	completeActivityTaskCmd   *redis.Script
	deleteInstanceCmd         *redis.Script
	futureEventsCmd           *redis.Script
	expireWorkflowInstanceCmd *redis.Script
	cancelWorkflowInstanceCmd *redis.Script
	signalWorkflowCmd         *redis.Script
)

func NewRedisBackend(client redis.UniversalClient, opts ...RedisBackendOption) (*redisBackend, error) {
	// Default options
	options := &RedisOptions{
		Options:      backend.ApplyOptions(),
		BlockTimeout: time.Second * 2,
	}

	for _, opt := range opts {
		opt(options)
	}

	ctx := context.Background()

	workflowQueue, err := newTaskQueue[workflowData](ctx, client, options.KeyPrefix, "workflows", options.WorkerName)
	if err != nil {
		return nil, ctxerrors.Wrap(err, "creating workflow task queue")
	}

	activityQueue, err := newTaskQueue[activityData](ctx, client, options.KeyPrefix, "activities", options.WorkerName)
	if err != nil {
		return nil, ctxerrors.Wrap(err, "creating activity task queue")
	}

	rb := &redisBackend{
		rdb:     client,
		options: options,
		keys:    newKeys(options.KeyPrefix),

		workflowQueue: workflowQueue,
		activityQueue: activityQueue,
	}

	// Load all Lua scripts
	cmdMapping := map[string]**redis.Script{
		"cancel_workflow_instance.lua": &cancelWorkflowInstanceCmd,
		"complete_activity_task.lua":   &completeActivityTaskCmd,
		"complete_workflow_task.lua":   &completeWorkflowTaskCmd,
		"create_workflow_instance.lua": &createWorkflowInstanceCmd,
		"delete_instance.lua":          &deleteInstanceCmd,
		"expire_workflow_instance.lua": &expireWorkflowInstanceCmd,
		"schedule_future_events.lua":   &futureEventsCmd,
		"signal_workflow.lua":          &signalWorkflowCmd,
	}

	if err := loadScripts(ctx, rb.rdb, cmdMapping); err != nil {
		return nil, ctxerrors.Wrap(err, "loading Lua scripts")
	}

	return rb, nil
}

func loadScripts(ctx context.Context, rdb redis.UniversalClient, cmdMapping map[string]**redis.Script) error {
	for scriptFile, cmd := range cmdMapping {
		scriptContent, err := fs.ReadFile(luaScripts, "scripts/"+scriptFile)
		if err != nil {
			return ctxerrors.Wrapf(err, "reading Lua script %s", scriptFile)
		}

		*cmd = redis.NewScript(string(scriptContent))

		if c := (*cmd).Load(ctx, rdb); c.Err() != nil {
			return ctxerrors.Wrapf(c.Err(), "loading Lua script %s", scriptFile)
		}
	}

	return nil
}

type redisBackend struct {
	rdb     redis.UniversalClient
	options *RedisOptions

	keys *keys

	workflowQueue *taskQueue[workflowData]
	activityQueue *taskQueue[activityData]
}

type workflowData struct{}

type activityData struct {
	Instance *core.WorkflowInstance `json:"instance,omitempty"`
	Queue    string                 `json:"queue,omitempty"`
	ID       string                 `json:"id,omitempty"`
	Event    *history.Event         `json:"event,omitempty"`
}

func (rb *redisBackend) Metrics() metrics.Client {
	return rb.options.Metrics.WithTags(metrics.Tags{metrickeys.Backend: "redis"})
}

func (rb *redisBackend) Tracer() trace.Tracer {
	return rb.options.TracerProvider.Tracer(backend.TracerName)
}

func (rb *redisBackend) Options() *backend.Options {
	return rb.options.Options
}

func (rb *redisBackend) Close() error {
	return rb.rdb.Close()
}

func (rb *redisBackend) FeatureSupported(feature backend.Feature) bool {
	switch feature {
	case backend.Feature_Expiration:
		return false
	}

	return true
}
