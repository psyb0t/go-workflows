package monoprocess

import (
	"context"

	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/ctxerrors/commerr"

	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/diag"
)

var _ diag.Backend = (*monoprocessBackend)(nil)

func (b *monoprocessBackend) GetWorkflowInstance(ctx context.Context, instance *core.WorkflowInstance) (*diag.WorkflowInstanceRef, error) {
	if diagBackend, ok := b.Backend.(diag.Backend); ok {
		return diagBackend.GetWorkflowInstance(ctx, instance)
	}
	return nil, ctxerrors.Wrap(commerr.ErrNotImplemented, "not implemented")
}

func (b *monoprocessBackend) GetWorkflowInstances(ctx context.Context, afterInstanceID, afterExecutionID string, count int) ([]*diag.WorkflowInstanceRef, error) {
	if diagBackend, ok := b.Backend.(diag.Backend); ok {
		return diagBackend.GetWorkflowInstances(ctx, afterInstanceID, afterExecutionID, count)
	}
	return nil, ctxerrors.Wrap(commerr.ErrNotImplemented, "not implemented")
}

func (b *monoprocessBackend) GetWorkflowTree(ctx context.Context, instance *core.WorkflowInstance) (*diag.WorkflowInstanceTree, error) {
	if diagBackend, ok := b.Backend.(diag.Backend); ok {
		return diagBackend.GetWorkflowTree(ctx, instance)
	}
	return nil, ctxerrors.Wrap(commerr.ErrNotImplemented, "not implemented")
}
