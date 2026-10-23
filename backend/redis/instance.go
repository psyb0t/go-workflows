package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/psyb0t/ctxerrors"

	"github.com/redis/go-redis/v9"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/backend/metadata"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/workflow"
)

func (rb *redisBackend) CreateWorkflowInstance(ctx context.Context, instance *workflow.Instance, event *history.Event) error {
	a := event.Attributes.(*history.ExecutionStartedAttributes)

	instanceState, err := json.Marshal(&instanceState{
		Queue:     string(a.Queue),
		Instance:  instance,
		State:     core.WorkflowInstanceStateActive,
		Metadata:  a.Metadata,
		CreatedAt: time.Now(),
	})
	if err != nil {
		return ctxerrors.Wrap(err, "marshaling instance state")
	}

	activeInstance, err := json.Marshal(instance)
	if err != nil {
		return ctxerrors.Wrap(err, "marshaling instance")
	}

	eventData, payloadData, err := marshalEvent(event)
	if err != nil {
		return ctxerrors.Wrap(err, "execute instance")
	}

	keyInfo := rb.workflowQueue.Keys(a.Queue)
	_, err = createWorkflowInstanceCmd.Run(ctx, rb.rdb, []string{
		rb.keys.instanceKey(instance),
		rb.keys.activeInstanceExecutionKey(instance.InstanceID),
		rb.keys.pendingEventsKey(instance),
		rb.keys.payloadKey(instance),
		rb.keys.instancesActive(),
		rb.keys.instancesByCreation(),
		keyInfo.SetKey,
		keyInfo.StreamKey,
		rb.workflowQueue.queueSetKey,
	},
		instanceSegment(instance),
		string(instanceState),
		string(activeInstance),
		event.ID,
		eventData,
		payloadData,
		time.Now().UTC().UnixNano(),
	).Result()
	if err != nil {
		var redisErr redis.Error
		if errors.As(err, &redisErr) {
			if redisErr.Error() == "ERR InstanceAlreadyExists" {
				return backend.ErrInstanceAlreadyExists
			}
		}

		return ctxerrors.Wrap(err, "creating workflow instance")
	}

	return nil
}

func (rb *redisBackend) GetWorkflowInstanceHistory(ctx context.Context, instance *core.WorkflowInstance, lastSequenceID *int64) ([]*history.Event, error) {
	start := "-"

	if lastSequenceID != nil {
		start = fmt.Sprintf("(%d", *lastSequenceID)
	}

	msgs, err := rb.rdb.XRange(ctx, rb.keys.historyKey(instance), start, "+").Result()
	if err != nil {
		return nil, ctxerrors.Wrap(err, "execute instance")
	}

	payloadKeys := make([]string, 0, len(msgs))
	events := make([]*history.Event, 0, len(msgs))
	for _, msg := range msgs {
		var event *history.Event
		if err := json.Unmarshal([]byte(msg.Values["event"].(string)), &event); err != nil {
			return nil, ctxerrors.Wrap(err, "unmarshaling event")
		}

		payloadKeys = append(payloadKeys, event.ID)
		events = append(events, event)
	}

	if len(payloadKeys) > 0 {
		res, err := rb.rdb.HMGet(ctx, rb.keys.payloadKey(instance), payloadKeys...).Result()
		if err != nil {
			return nil, ctxerrors.Wrap(err, "reading payloads")
		}

		for i, event := range events {
			event.Attributes, err = history.DeserializeAttributes(event.Type, []byte(res[i].(string)))
			if err != nil {
				return nil, ctxerrors.Wrapf(err, "deserializing attributes for event %v", event.Type)
			}
		}
	}

	return events, nil
}

func (rb *redisBackend) GetWorkflowInstanceState(ctx context.Context, instance *core.WorkflowInstance) (core.WorkflowInstanceState, error) {
	instanceState, err := readInstance(ctx, rb.rdb, rb.keys.instanceKey(instance))
	if err != nil {
		return core.WorkflowInstanceStateActive, ctxerrors.Wrap(err, "execute instance")
	}

	return instanceState.State, nil
}

func (rb *redisBackend) CancelWorkflowInstance(ctx context.Context, instance *core.WorkflowInstance, event *history.Event) error {
	// Read the instance to check if it exists
	instanceState, err := readInstance(ctx, rb.rdb, rb.keys.instanceKey(instance))
	if err != nil {
		return ctxerrors.Wrap(err, "execute instance")
	}

	// Prepare event data
	eventData, payloadData, err := marshalEvent(event)
	if err != nil {
		return ctxerrors.Wrap(err, "execute instance")
	}

	keyInfo := rb.workflowQueue.Keys(workflow.Queue(instanceState.Queue))

	// Cancel instance
	if err := cancelWorkflowInstanceCmd.Run(ctx, rb.rdb, []string{
		rb.keys.payloadKey(instance),
		rb.keys.pendingEventsKey(instance),
		keyInfo.SetKey,
		keyInfo.StreamKey,
	},
		event.ID,
		eventData,
		payloadData,
		instanceSegment(instance),
	).Err(); err != nil {
		return ctxerrors.Wrap(err, "canceling workflow instance")
	}

	return nil
}

func (rb *redisBackend) RemoveWorkflowInstance(ctx context.Context, instance *core.WorkflowInstance) error {
	i, err := readInstance(ctx, rb.rdb, rb.keys.instanceKey(instance))
	if err != nil {
		return ctxerrors.Wrap(err, "execute instance")
	}

	if i.State != core.WorkflowInstanceStateFinished && i.State != core.WorkflowInstanceStateContinuedAsNew {
		return backend.ErrInstanceNotFinished
	}

	return rb.deleteInstance(ctx, instance)
}

func (rb *redisBackend) RemoveWorkflowInstances(ctx context.Context, options ...backend.RemovalOption) error {
	return backend.ErrNotSupported{
		Message: "not supported, use auto-expiration",
	}
}

type instanceState struct {
	Queue string `json:"queue"`

	Instance *core.WorkflowInstance     `json:"instance,omitempty"`
	State    core.WorkflowInstanceState `json:"state,omitempty"`

	Metadata *metadata.WorkflowMetadata `json:"metadata,omitempty"`

	CreatedAt   time.Time  `json:"created_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	LastSequenceID int64 `json:"last_sequence_id,omitempty"`
}

func readInstance(ctx context.Context, rdb redis.UniversalClient, instanceKey string) (*instanceState, error) {
	p := rdb.Pipeline()

	cmd := readInstanceP(ctx, p, instanceKey)

	// Error is checked when checking the cmd
	_, _ = p.Exec(ctx)

	return readInstancePipelineCmd(cmd)
}

func readInstanceP(ctx context.Context, p redis.Pipeliner, instanceKey string) *redis.StringCmd {
	return p.Get(ctx, instanceKey)
}

func readInstancePipelineCmd(cmd *redis.StringCmd) (*instanceState, error) {
	val, err := cmd.Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, backend.ErrInstanceNotFound
		}

		return nil, ctxerrors.Wrap(err, "reading instance")
	}

	var state instanceState
	if err := json.Unmarshal([]byte(val), &state); err != nil {
		return nil, ctxerrors.Wrap(err, "unmarshaling instance state")
	}

	return &state, nil
}

func (rb *redisBackend) readActiveInstanceExecution(ctx context.Context, instanceID string) (*core.WorkflowInstance, error) {
	val, err := rb.rdb.Get(ctx, rb.keys.activeInstanceExecutionKey(instanceID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}

		if err != nil {
			return nil, ctxerrors.Wrap(err, "execute instance")
		}

		return nil, nil
	}

	var instance *core.WorkflowInstance
	if err := json.Unmarshal([]byte(val), &instance); err != nil {
		return nil, ctxerrors.Wrap(err, "unmarshaling instance")
	}

	return instance, nil
}
