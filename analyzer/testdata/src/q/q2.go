// nolint
package q

import (
	wf "github.com/psyb0t/go-workflows/workflow"
)

func wfWrongOrder2(ctx wf.Context) (error, string) { // want "workflow `wfWrongOrder2` doesn't return `error` as last return value"
	return nil, ""
}
