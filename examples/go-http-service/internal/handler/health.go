package handler

import (
	"fmt"
	"net/http"

	"github.com/lotosli/sandbox-runner/pkg/helper"
)

func Health(w http.ResponseWriter, r *http.Request) {
	ctx, span := helper.StartSpan(r.Context(), "health.check")
	defer span.End()
	helper.AddEvent(ctx, "health.response")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintln(w, "ok")
}
