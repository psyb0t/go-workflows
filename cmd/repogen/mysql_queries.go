package main

import (
	"time"

	"gorm.io/gen"

	"github.com/psyb0t/go-workflows/backend/mysql/db/models"
)

type MysqlEventQuerier interface {
	/*
	   SELECT e.event_id AS id, e.sequence_id, e.instance_id, e.execution_id,
	       e.event_type, e.timestamp, e.schedule_event_id, e.visible_at, a.data
	   FROM @@table e LEFT JOIN attributes a ON a.event_id = e.event_id
	       AND a.instance_id = e.instance_id AND a.execution_id = e.execution_id
	   WHERE e.instance_id = @instanceID AND e.execution_id = @executionID
	       AND e.sequence_id > @afterSequence
	       AND (NOT @pending OR e.visible_at IS NULL OR e.visible_at <= @now)
	   ORDER BY CASE WHEN @pending THEN e.id ELSE e.sequence_id END, e.id
	*/
	EventsWithAttributes(instanceID, executionID string, afterSequence int64, pending bool, now time.Time) ([]*models.EventPayload, error)

	/*
	   SELECT e.event_id AS id, e.sequence_id, e.instance_id, e.execution_id,
	       e.event_type, e.timestamp, e.schedule_event_id, e.visible_at, a.data
	   FROM @@table e LEFT JOIN attributes a ON a.event_id = e.event_id
	       AND a.instance_id = e.instance_id AND a.execution_id = e.execution_id
	   WHERE e.visible_at IS NOT NULL ORDER BY e.id
	*/
	FutureWithAttributes() ([]*models.EventPayload, error)
}

type MysqlInstanceQueries interface {
	/*
	   SELECT i.* FROM @@table i
	   INNER JOIN instances anchor ON anchor.instance_id = @afterID AND anchor.execution_id = @afterExecution
	   WHERE i.created_at < anchor.created_at
	      OR (i.created_at = anchor.created_at AND i.instance_id < anchor.instance_id)
	      OR (i.created_at = anchor.created_at AND i.instance_id = anchor.instance_id
	          AND i.execution_id < anchor.execution_id)
	   ORDER BY i.created_at DESC, i.instance_id DESC, i.execution_id DESC LIMIT @limit
	*/
	PageAfter(afterID, afterExecution string, limit int) ([]*gen.T, error)

	/*
	   SELECT i.queue, COUNT(*) AS count FROM @@table i
	   WHERE i.state = @state AND i.completed_at IS NULL
	     AND (i.locked_until IS NULL OR i.locked_until < @now)
	     AND EXISTS (SELECT 1 FROM pending_events e
	         WHERE e.instance_id = i.instance_id AND e.execution_id = i.execution_id
	         AND (e.visible_at IS NULL OR e.visible_at <= @now))
	   GROUP BY i.queue
	*/
	RunnableCounts(state int, now time.Time) ([]*models.QueueCount, error)
}
