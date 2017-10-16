package rotary_test

import (
	"sync"
	"testing"
	"time"

	. "github.com/rickn42/rotary"
	"github.com/stretchr/testify/assert"
)

type dummyEvent struct {
	handled bool
}

func (e *dummyEvent) Handle(ctx Context) error {
	e.handled = true
	return nil
}

func TestRotary_NotOpen(t *testing.T) {

	rot := NewRotary(10)
	ctx := rot.NewRotaryContext()
	result := rot.Send(ctx, &dummyEvent{})

	// if rotary is not opened,
	// result is instantly done with an error.
	select {
	case <-result.Done():
	default:
		t.Errorf("Not openned rotary result should be done immediatly")
	}

	assert.Equal(t, result.Err(), NotOpened)
}

func TestRotary_Send(t *testing.T) {

	rot := NewRotary(100)
	rot.Open(10)

	evt := &dummyEvent{}
	ctx := rot.NewRotaryContext()
	err := rot.Send(ctx, evt).Err()

	assert.Nil(t, err)
	assert.True(t, evt.handled)
}

func TestRotary_RotaryContext(t *testing.T) {

	rot := NewRotary(10)
	ctx := rot.NewRotaryContext()
	assert.Equal(t, ctx.Rotary(), rot, "Context rotary should be that rotary instance.")
}

type topEvent struct {
	sub     *subEvent
	handled bool
}

func (e *topEvent) Handle(ctx Context) error {
	ctx.Rotary().Send(ctx, e.sub) // no wait subEvent
	e.handled = true
	return nil
}

type subEvent struct {
	wait    time.Duration
	handled bool
}

func (e *subEvent) Handle(ctx Context) error {
	time.Sleep(e.wait)
	e.handled = true
	return nil
}

func TestRotary_waitGroupContext(t *testing.T) {

	rot := NewRotary(100)
	rot.Open(10)

	ctx := rot.NewRotaryContext()
	ctx = WithWaitGroup(ctx, &sync.WaitGroup{})

	subEvt := &subEvent{wait: 100 * time.Millisecond}
	topEvt := &topEvent{sub: subEvt}
	res := rot.Send(ctx, topEvt)
	res.Wait()

	// topEvent is done.
	// but, subEvent is working yet.
	assert.True(t, res.IsDone())
	assert.True(t, topEvt.handled)
	assert.False(t, subEvt.handled)

	// wait for subEvent done
	ctx.WaitSubEvent()
	// now, subEvt is done.
	assert.True(t, subEvt.handled)
}

type callerEvent struct {
	callee  *calleeEvent
	handled bool
}

func (e *callerEvent) Handle(ctx Context) error {
	ctx.AddCallbackEvent(ctx, e.callee)
	e.handled = true
	return nil
}

type calleeEvent struct {
	handled bool
}

func (e *calleeEvent) Handle(ctx Context) error {
	e.handled = true
	return nil
}

func TestRotary_callbackContext(t *testing.T) {

	rot := NewRotary(100)
	rot.Open(10)

	ctx := rot.NewRotaryContext()
	ctx = WithCallback(ctx)

	callee := &calleeEvent{}
	caller := &callerEvent{callee: callee}
	rot.Send(ctx, caller).Wait()

	assert.True(t, caller.handled)
	assert.False(t, callee.handled)

	for _, evt := range ctx.CallbackEvents() {
		rot.Send(ctx, evt).Wait()
	}

	assert.True(t, caller.handled)
	assert.True(t, callee.handled)
}

type reporterCtx struct {
	Context
	reported *interface{}
}

func (r reporterCtx) Report(_ Context, data interface{}) error {
	*r.reported = data
	return nil
}

type reporterEvent struct {
	report  interface{}
	handled bool
}

func (e *reporterEvent) Handle(ctx Context) error {
	ctx.Report(ctx, e.report)
	e.handled = true
	return nil
}

func TestRotary_reportContext(t *testing.T) {

	rot := NewRotary(100)
	rot.Open(10)

	ctx := rot.NewRotaryContext()
	var reported interface{}
	ctx = reporterCtx{Context: ctx, reported: &reported}

	var report interface{} = 42
	rot.Send(ctx, &reporterEvent{report: report}).Wait()

	assert.Equal(t, reported, 42)
}
