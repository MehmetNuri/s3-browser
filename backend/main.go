package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime/debug"
	"s3browser/internal/desktop"
	"sync"
)

//go:embed api-methods.json
var methodList []byte

// allowedMethods returns every dispatchable method. The desktop host decides
// which of them the renderer may call; "host" methods take local paths or
// change desktop state and are reserved for the main process.
func allowedMethods() (map[string]bool, error) {
	var lists struct {
		Renderer []string `json:"renderer"`
		Host     []string `json:"host"`
	}
	if err := json.Unmarshal(methodList, &lists); err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(lists.Renderer)+len(lists.Host))
	for _, name := range append(lists.Renderer, lists.Host...) {
		allowed[name] = true
	}
	return allowed, nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host := desktop.NewHost(os.Stdout)
	a := NewApp()
	a.startup(desktop.WithHost(ctx, host))
	allowed, err := allowedMethods()
	if err != nil {
		panic(err)
	}
	_ = host.Send(desktop.Message{Type: "ready"})
	var workers sync.WaitGroup
	semaphore := make(chan struct{}, 64)
	err = host.Read(os.Stdin, func(request desktop.Message) {
		if !allowed[request.Method] {
			_ = host.Send(desktop.Message{Type: "response", ID: request.ID, Error: "Unknown operation"})
			return
		}
		select {
		case semaphore <- struct{}{}:
		default:
			_ = host.Send(desktop.Message{Type: "response", ID: request.ID, Error: "Too many active operations"})
			return
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-semaphore }()
			result, err := invoke(a, request.Method, request.Args)
			response := desktop.Message{Type: "response", ID: request.ID, Result: result}
			if err != nil {
				response.Error = err.Error()
			}
			_ = host.Send(response)
		}()
	})
	cancel()
	workers.Wait()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func invoke(app *App, name string, args []json.RawMessage) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintf(os.Stderr, "panic in %s: %v\n%s", name, recovered, debug.Stack())
			err = fmt.Errorf("Operation failed: %v", recovered)
		}
	}()
	method := reflect.ValueOf(app).MethodByName(name)
	if !method.IsValid() || len(args) != method.Type().NumIn() {
		return nil, fmt.Errorf("Invalid arguments for %s", name)
	}
	inputs := make([]reflect.Value, len(args))
	for i, arg := range args {
		value := reflect.New(method.Type().In(i))
		if err := json.Unmarshal(arg, value.Interface()); err != nil {
			return nil, fmt.Errorf("Invalid argument %d for %s", i+1, name)
		}
		inputs[i] = value.Elem()
	}
	outputs := method.Call(inputs)
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	for _, output := range outputs {
		if output.Type().Implements(errorType) {
			if !output.IsNil() {
				return nil, output.Interface().(error)
			}
		} else {
			result = output.Interface()
		}
	}
	return result, nil
}
