/*
 * Copyright (c) 2021, Oracle and/or its affiliates.
 * Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
 */

package utils

import (
	"context"
	"sync"
)

type Bolt struct {
	mutex	sync.Mutex
	cv		*sync.Cond
	context	context.Context
}

func NewBolt() *Bolt {
	bolt := new(Bolt)
	bolt.cv = sync.NewCond(&bolt.mutex)
	return bolt
}

func (l *Bolt) Lock(ctx context.Context) {
	l.mutex.Lock()
	for l.context != nil {
		l.cv.Wait()
	}
	l.context = ctx
	l.mutex.Unlock()
}

func (l *Bolt) Unlock(ctx context.Context) {
	l.mutex.Lock()
	if l.context != ctx {
		panic("wrong owner unlocking fs")
	}
	l.context = nil
	l.cv.Signal()
	l.mutex.Unlock()
}
