package httpapi

import "sync/atomic"

type Readiness struct {
	ready atomic.Bool
}

func (r *Readiness) Set(val bool) {
	r.ready.Store(val)
}

func (r *Readiness) IsReady() bool {
	return r.ready.Load()
}
