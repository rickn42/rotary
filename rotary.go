package rotary

import (
	"fmt"
	"runtime/debug"
	"sync"

	"runtime"

	"github.com/pkg/errors"
	. "github.com/rickn42/result"
)

var NotOpened = errors.New("Rotary not opened.")

type Event interface {
	Handle(ctx Context) error
}

type vehicle struct {
	ctx      Context
	event    Event
	result   Result
	callback func()
}

type Rotary interface {
	NewRotaryContext() Context
	Send(Context, Event) Result
	Handle(Context, Event, Result, func())
	Loop(int)
	Open(int)
	SetDevMode(bool)
	SetLogger(logger)
}

type logger interface {
	Info(string)
	Infof(string, ...interface{})
	Error(string)
	Errorf(string, ...interface{})
}

type rotary struct {
	circle chan vehicle
	mu     sync.Mutex
	open   bool
	dev    bool
	logger logger
}

func NewRotary(bufSize int) Rotary {
	return &rotary{
		circle: make(chan vehicle, bufSize),
	}
}

func (r *rotary) NewRotaryContext() Context {
	return WithRotary(new(emptyCtx), r)
}

func (r *rotary) SetDevMode(flag bool) {
	r.dev = flag
}

func (r *rotary) SetLogger(log logger) {
	r.logger = log
}

func (r *rotary) Send(ctx Context, e Event) Result {
	res := NewResult()

	if !r.open {
		res.Set(NotOpened)
		return res
	}

	ctx.AddSubEvent()

	wgCtx := WithWaitGroup(ctx, &sync.WaitGroup{})
	r.circle <- vehicle{
		ctx:      wgCtx,
		event:    e,
		result:   res,
		callback: ctx.DoneSubEvent,
	}

	return res
}

func (r *rotary) Handle(ctx Context, e Event, res Result, cb func()) {
	var err error

	if r.dev {
		r.logger.Infof("rotary.Handle: %s\n", e)
	}

	defer func() {
		recovered := recover()

		if recovered != nil {
			trace := "panic dev.Stack():\n" + string(debug.Stack())

			var err2 error
			if e, ok := recovered.(error); ok {
				err2 = e
			} else {
				err2 = errors.New(fmt.Sprintf("%v", e))
			}

			if err == nil {
				err = err2
			} else {
				err = errors.Wrap(err, err2.Error())
			}

			err = errors.Wrap(err, trace)
		}

		res.Set(err)
		cb()
	}()

	err = e.Handle(ctx)
}

func (r *rotary) Loop(outletCnt int) {
	r.mu.Lock()

	if r.open {
		return
	}
	r.open = true

	r.mu.Unlock()

	r.loop(outletCnt)
}

func (r *rotary) loop(outletCnt int) {
	var sem = make(chan int, outletCnt)
	for {
		sem <- 1
		v := <-r.circle
		go r.Handle(v.ctx, v.event, v.result, v.callback)
		<-sem
	}
}

func (r *rotary) Open(outletCnt int) {
	go r.Loop(outletCnt)
	runtime.Gosched()
}
