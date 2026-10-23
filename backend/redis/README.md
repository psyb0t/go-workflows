# Redis backend

Redis stores execution state and histories and coordinates workflow/activity consumers through streams and Lua scripts. See [backend operations](../../docs/backends.md) for deployment and durability requirements.

## Client ownership and retention

`NewRedisBackend` accepts an application-configured `redis.UniversalClient`. Configure authentication, TLS and connection policy in that client. The backend's `Close` closes the supplied Redis client; do not share it with an application component that must outlive the backend.

`WithKeyPrefix` separates key namespaces, not authorization. Use separate credentials or Redis deployments when access must be isolated. `WithBlockTimeout` controls blocking queue reads.

Finished executions do not expire by default. Configure `WithAutoExpiration` for finished runs and `WithAutoExpirationContinueAsNew` for continued executions. The client-managed SQL expiration workflow is not supported by Redis; `Feature_Expiration` reports false. Redis persistence and replication settings determine whether stored state survives server failure.

## Instances and their state

Execution state uses `instance:{instanceID}:{executionID}` keys, preceded by the configured key prefix. Additional indexes track active instances, creation order and expiration. `keys.go` owns the exact names; do not construct or mutate them in application code.

## History and pending events

Recorded history and pending events use separate per-execution streams, keyed by `history:{instanceID}:{executionID}` and `pending-events:{instanceID}:{executionID}`, with the configured prefix. The worker replays recorded events and consumes pending events when processing the next workflow task.

## Timer events

Timer events are stored in a sorted set (`ZSET`). Whenever a worker checks for a new workflow instance task, the sorted set is checked to see if any of the pending timer events is ready yet. If it is, it's added to the pending events before those are returned for pending workflow tasks.

## Task queues

We need queues for activities and workflow instances. In both cases, we have tasks being enqueued, workers polling for works, and we have to guarantee that every task is eventually processed. So if a worker has dequeued a task and crashed, for example, eventually we need another worker to pick up the task and finish it.

Task queues are implemented using Redis STREAMs. In addition for queues where we only want a single instance of a task to be in the queue, we maintain an additional `SET`.

<details>
  <summary>Alternatives considered</summary>

  **Option 1 - `ZSET`**:

  Store keys for queue items in a `ZSET`. The score for the sorted set is the timestamp when the task is unlocked. For new tasks the `SCORE` is the current timestamp.

  Workers make a `ZRANGE` query to that sorted set, looking for tasks where the `SCORE` is in `-inf, now)`. That finds tasks whose unlock timestamp is in the past, either because they were just queued or because the lock expired.

  Once a worker picks up a task, the score is updated to `now + lock_timeout`. Query and update are done in a transaction with a `WATCH` on the queue key.

  When a worker is done with a task, it removes it from the `ZSET` (`ZREM`).

  Pro:
  - No special handling for recovering crashed tasks, they'll automatically unlock

  Con:
  - Need for polling and cannot use any of the blocking redis commands
  - WAIT with transaction, or a script required

  **Option 2 - `LISTS`**

  Use two `LIST`s, one for the _pending_ queue, one for the _processing_ queue. To enqueue a new task, `LPUSH` it onto the _pending_ list. Also add an entry in a separate `ZSET` where the score is the unlock timestamp. Initially that timestamp will be the current timestamp. The `LPUSH` and `ZADD` are done in a transaction with a `WATCH` on the queue key, and retried if another client modified the queue in the mean time. Alternatively, the two operations can be done in a script.

  For picking up tasks, we use a blocking `BLMOVE .. RIGHT  LEFT` command to pick up the next available task from _pending_ and move it to _processing_ as a single atomic operation. Once picked up, the `SCORE` in the `ZSET` is adjusted to `now + lock_timeout`.

  When a worker is done with a task, it removes it from the _processing_ list (`LREM`), and the `ZSET` (`ZREM`).

  To recover abandoned tasks, we periodically scan the _processing_ list

  Pro:
  - Blocking call does not require constant polling

  Con:
  - Requires periodic scans of the _processing_ list to find tasks that have been abandoned
  - Picking up a task, adjusting its ZSET value and the periodic scan could run into race conditions

</details>
