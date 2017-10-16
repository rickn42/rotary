package rotary

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

// TODO refactoring context (context.Context copy code...)

// Context extends context.Context with some features.
type Context interface {
	Deadline() (deadline time.Time, ok bool)
	Done() <-chan struct{}
	Err() error
	Value(key interface{}) interface{}
	waitGroupContext
	rotaryContext
	callbackContext
	ReporterContext
}

type ReporterContext interface {
	Report(Context, interface{}) error
}

type CallbackEvent struct {
	Context
	Event
}

type callbackContext interface {
	AddCallbackEvent(Context, Event)
	CallbackEvents() []CallbackEvent
}

func TestCallbackContext(t *testing.T) {
	var _ callbackContext = new(callbackCtx)
}

type callbackCtx struct {
	Context
	cs []CallbackEvent
}

func (c *callbackCtx) AddCallbackEvent(ctx Context, e Event) {
	c.cs = append(c.cs, CallbackEvent{ctx, e})
}

func (c *callbackCtx) CallbackEvents() []CallbackEvent {
	return c.cs
}

func WithCallback(parent Context) Context {
	return &callbackCtx{parent, []CallbackEvent{}}
}

// waitGroupContext is for trace whether sub-context routine is done.
type waitGroupContext interface {
	AddSubEvent()
	DoneSubEvent()
	WaitSubEvent()
}

func WithWaitGroup(parent Context, wg *sync.WaitGroup) Context {
	return &waitGroupCtx{
		Context: parent,
		wg:      wg,
	}
}

func TestWaitGroupContext(t *testing.T) {
	var _ waitGroupContext = new(waitGroupCtx)
}

type waitGroupCtx struct {
	Context
	wg *sync.WaitGroup
}

func (w *waitGroupCtx) AddSubEvent() {
	w.wg.Add(1)
	w.Context.AddSubEvent()
}

func (w *waitGroupCtx) DoneSubEvent() {
	w.wg.Done()
	w.Context.DoneSubEvent()
}

func (w *waitGroupCtx) WaitSubEvent() {
	w.wg.Wait()
}

type rotaryContext interface {
	Rotary() Rotary
}

func WithRotary(parent Context, rot Rotary) Context {
	return &rotaryCtx{parent, rot}
}

func TestRotaryContext(t *testing.T) {
	var _ rotaryContext = new(rotaryCtx)
}

type rotaryCtx struct {
	Context
	rot Rotary
}

func (r *rotaryCtx) Rotary() Rotary {
	return r.rot
}

func TestEmtpyCtx(t *testing.T) {
	var _ Context = new(emptyCtx)
}

// Implement emptyCtx methods with Context interface
func (*emptyCtx) AddSubEvent() {
	return
}

func (*emptyCtx) DoneSubEvent() {
	return
}

func (*emptyCtx) WaitSubEvent() {
	return
}

func (*emptyCtx) Rotary() Rotary {
	return nil
}

func (*emptyCtx) AddCallbackEvent(Context, Event) {
	return
}

func (*emptyCtx) CallbackEvents() []CallbackEvent {
	return nil
}

func (*emptyCtx) Report(Context, interface{}) error {
	return nil
}

//////////////////////////////////////////////////
// ALL CODE BELLOW IS COPY FROM context library.
// only replace context.Context with rotary.Context.

var Canceled = errors.New("context canceled")

var DeadlineExceeded error = deadlineExceededError{}

type deadlineExceededError struct{}

func (deadlineExceededError) Error() string   { return "context deadline exceeded" }
func (deadlineExceededError) Timeout() bool   { return true }
func (deadlineExceededError) Temporary() bool { return true }

type emptyCtx int

func (*emptyCtx) Deadline() (deadline time.Time, ok bool) {
	return
}

func (*emptyCtx) Done() <-chan struct{} {
	return nil
}

func (*emptyCtx) Err() error {
	return nil
}

func (*emptyCtx) Value(key interface{}) interface{} {
	return nil
}

func (e *emptyCtx) String() string {
	switch e {
	case background:
		return "context.Background"
	case todo:
		return "context.TODO"
	}
	return "unknown empty Context"
}

var (
	background = new(emptyCtx)
	todo       = new(emptyCtx)
)

//func Background() Context {
//	return background
//}

//func TODO() Context {
//	return todo
//}

type CancelFunc func()

func WithCancel(parent Context) (ctx Context, cancel CancelFunc) {
	c := newCancelCtx(parent)
	propagateCancel(parent, &c)
	return &c, func() { c.cancel(true, Canceled) }
}

func newCancelCtx(parent Context) cancelCtx {
	return cancelCtx{
		Context: parent,
		done:    make(chan struct{}),
	}
}

func propagateCancel(parent Context, child canceler) {
	if parent.Done() == nil {
		return // parent is never canceled
	}
	if p, ok := parentCancelCtx(parent); ok {
		p.mu.Lock()
		if p.err != nil {
			// parent has already been canceled
			child.cancel(false, p.err)
		} else {
			if p.children == nil {
				p.children = make(map[canceler]struct{})
			}
			p.children[child] = struct{}{}
		}
		p.mu.Unlock()
	} else {
		go func() {
			select {
			case <-parent.Done():
				child.cancel(false, parent.Err())
			case <-child.Done():
			}
		}()
	}
}

func parentCancelCtx(parent Context) (*cancelCtx, bool) {
	for {
		switch c := parent.(type) {
		case *cancelCtx:
			return c, true
		case *timerCtx:
			return &c.cancelCtx, true
		case *valueCtx:
			parent = c.Context
		default:
			return nil, false
		}
	}
}

func removeChild(parent Context, child canceler) {
	p, ok := parentCancelCtx(parent)
	if !ok {
		return
	}
	p.mu.Lock()
	if p.children != nil {
		delete(p.children, child)
	}
	p.mu.Unlock()
}

type canceler interface {
	cancel(removeFromParent bool, err error)
	Done() <-chan struct{}
}

type cancelCtx struct {
	Context

	done chan struct{} // closed by the first cancel call.

	mu       sync.Mutex
	children map[canceler]struct{} // set to nil by the first cancel call
	err      error                 // set to non-nil by the first cancel call
}

func (c *cancelCtx) Done() <-chan struct{} {
	return c.done
}

func (c *cancelCtx) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *cancelCtx) String() string {
	return fmt.Sprintf("%v.WithCancel", c.Context)
}

func (c *cancelCtx) cancel(removeFromParent bool, err error) {
	if err == nil {
		panic("context: internal error: missing cancel error")
	}
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return // already canceled
	}
	c.err = err
	close(c.done)
	for child := range c.children {
		// NOTE: acquiring the child's lock while holding parent's lock.
		child.cancel(false, err)
	}
	c.children = nil
	c.mu.Unlock()

	if removeFromParent {
		removeChild(c.Context, c)
	}
}

func WithDeadline(parent Context, deadline time.Time) (Context, CancelFunc) {
	if cur, ok := parent.Deadline(); ok && cur.Before(deadline) {
		// The current deadline is already sooner than the new one.
		return WithCancel(parent)
	}
	c := &timerCtx{
		cancelCtx: newCancelCtx(parent),
		deadline:  deadline,
	}
	propagateCancel(parent, c)
	d := time.Until(deadline)
	if d <= 0 {
		c.cancel(true, DeadlineExceeded) // deadline has already passed
		return c, func() { c.cancel(true, Canceled) }
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		c.timer = time.AfterFunc(d, func() {
			c.cancel(true, DeadlineExceeded)
		})
	}
	return c, func() { c.cancel(true, Canceled) }
}

type timerCtx struct {
	cancelCtx
	timer *time.Timer // Under cancelCtx.mu.

	deadline time.Time
}

func (c *timerCtx) Deadline() (deadline time.Time, ok bool) {
	return c.deadline, true
}

func (c *timerCtx) String() string {
	return fmt.Sprintf("%v.WithDeadline(%s [%s])", c.cancelCtx.Context, c.deadline, time.Until(c.deadline))
}

func (c *timerCtx) cancel(removeFromParent bool, err error) {
	c.cancelCtx.cancel(false, err)
	if removeFromParent {
		// Remove this timerCtx from its parent cancelCtx's children.
		removeChild(c.cancelCtx.Context, c)
	}
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.mu.Unlock()
}

func WithTimeout(parent Context, timeout time.Duration) (Context, CancelFunc) {
	return WithDeadline(parent, time.Now().Add(timeout))
}

func WithValue(parent Context, key, val interface{}) Context {
	if key == nil {
		panic("nil key")
	}
	if !reflect.TypeOf(key).Comparable() {
		panic("key is not comparable")
	}
	return &valueCtx{parent, key, val}
}

type valueCtx struct {
	Context
	key, val interface{}
}

func (c *valueCtx) String() string {
	return fmt.Sprintf("%v.WithValue(%#v, %#v)", c.Context, c.key, c.val)
}

func (c *valueCtx) Value(key interface{}) interface{} {
	if c.key == key {
		return c.val
	}
	return c.Context.Value(key)
}
