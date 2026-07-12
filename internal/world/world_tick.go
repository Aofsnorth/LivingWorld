package world

import "sync/atomic"

func (w *World) SetTime(t int64)    { atomic.StoreInt64(&w.time, t) }
func (w *World) GetTime() int64     { return atomic.LoadInt64(&w.time) }
func (w *World) SetDayTime(t int64) { atomic.StoreInt64(&w.dayTime, t) }
func (w *World) GetDayTime() int64  { return atomic.LoadInt64(&w.dayTime) }
