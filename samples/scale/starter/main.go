package main

import (
	"context"
	"flag"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/samples"
	scale "github.com/psyb0t/go-workflows/samples/scale"
)

var (
	tostart = flag.Int("count", 100, "Number of workflow instances to start")
	count   int32
)

func main() {
	ctx := context.Background()
	b := samples.GetBackend("scale", false)

	if *tostart < 0 || *tostart > math.MaxInt32 {
		panic("count must fit a nonnegative int32")
	}
	count = int32(*tostart)

	// Start workflow via client
	c := client.New(b)

	wg := &sync.WaitGroup{}

	now := time.Now()

	for i := 0; i < *tostart; i++ {
		wg.Add(1)

		go startWorkflow(ctx, c, wg)
	}

	wg.Wait()

	log.Println("Finished in", time.Since(now))
}

func startWorkflow(ctx context.Context, c *client.Client, wg *sync.WaitGroup) {
	wf, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, scale.Workflow1, "Hello world "+uuid.NewString())
	if err != nil {
		panic("could not start workflow")
	}

	err = c.WaitForWorkflowInstance(ctx, wf, time.Second*30)
	if err != nil {
		log.Println("Received error while waiting for workflow", wf.InstanceID, err)
	}

	cn := atomic.AddInt32(&count, -1)
	log.Print(cn)

	wg.Done()
}
