# Rotary

simple event loop system

### basic

rotary.Event interface 
```go
type Event interface{
        // warn: ctx is rotary.Context, not context.Context
        Handle(ctx Context) error
} 
```

### how to use

```go
rot := rotary.NewRotary(100) // event buffer size
rot.Open(10) // concurrent worker count

ctx := rot.NewRotaryContext() // default context

res := rot.Send(ctx, SomeEvent{})

// result 
<-res.Done()
val, err := res.Value()
```

### custom event 

```go
type SomeEvent struct {}

func (*SomeEvent) Handle(ctx rotary.Context) error {
        res := ctx.Rotary().Send(ctx, &AnotherEvent{})
        // ... 
        return nil  
}
```

### waitgroup context 
wait for all sub-event done.

`ctx.WaitSubEvent()`

```go
ctx := rot.NewRotaryContext()
ctx = rotary.WithWaitGroup(ctx, &sync.WaitGroup{})

// this call many nested sub events.
evt := &nestedEvent{} 
res := rot.Send(ctx, evt)

// wait for only that event done.
res.Wait()

// wait for not only top-event done,
// but also all sub-events working done. 
ctx.WaitSubEvent()
```

### callback event context 
add callback event and do it later.

`ctx.AddCallbackEvent(ctx Context, cb Event)`
 
`ctx.CallbackEvents() []CallbackEvent` 

```go
rot := NewRotary(100)
rot.Open(10)

ctx := rot.NewRotaryContext()
ctx = WithCallback(ctx)

rot.Send(ctx, CallerEvent{
        handle: func(ctx rotary.Context) error {
                ctx.AddCallbackEvent(ctx, CallbackEvent{})
                return nil 
        }
}).Wait()

for _, callbackEvt := range ctx.CallbackEvents() {
		rot.Send(ctx, callbackEvt).Wait()
}
```

### report context
break call without event layer.

`ctx.Report(ctx Context, data interface{}) error`

first, make report context structure.
```go
type reporterContext struct {
	    Context
}

func (r reporterContext) Report(_ Context, data interface{}) error {
	    fmt.Println("report data:", data)
	    return nil
}
```

then, wrap ctx with it.

```go
rot := NewRotary(100)
rot.Open(10)

ctx := rot.NewRotaryContext()
ctx = reporterContext{ctx}

rot.Send(ctx, ReportEvent{
        handle: func(ctx rotary.Context) error {
                ctx.Report(ctx, 42)
                return nil 
        }
}).Wait()

// report data: 42
```