package main

import (
	"time"

	"gorm.io/gen"
)

type SQLiteInstanceQuerier interface {
	/*
	   SELECT i.* FROM @@table i
	   WHERE i.state = @state AND i.completed_at IS NULL
	     AND (i.locked_until IS NULL OR i.locked_until < @now)
	     AND (i.sticky_until IS NULL OR i.sticky_until < @now OR i.worker = @worker)
	     AND i.queue IN @queues
	     AND EXISTS (SELECT 1 FROM pending_events pe
	         WHERE pe.instance_id = i.id AND pe.execution_id = i.execution_id
	         AND (pe.visible_at IS NULL OR pe.visible_at <= @now))
	   ORDER BY i.rowid LIMIT 1
	*/
	NextRunnable(now time.Time, worker string, queues []string, state int) (*gen.T, error)
}

type ServerInstanceQuerier interface {
	/*
	   SELECT i.* FROM @@table i
	   WHERE i.state = @state AND i.completed_at IS NULL
	     AND (i.locked_until IS NULL OR i.locked_until < @now)
	     AND (i.sticky_until IS NULL OR i.sticky_until < @now OR i.worker = @worker)
	     AND i.queue IN @queues
	     AND EXISTS (SELECT 1 FROM pending_events pe
	         WHERE pe.instance_id = i.instance_id AND pe.execution_id = i.execution_id
	         AND (pe.visible_at IS NULL OR pe.visible_at <= @now))
	   ORDER BY i.id LIMIT 1 FOR UPDATE OF i SKIP LOCKED
	*/
	NextRunnable(now time.Time, worker string, queues []string, state int) (*gen.T, error)
}
