// Copyright (c) 2019 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package parallel

import (
	"bufio"
	"bytes"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBasic(t *testing.T) {
	cmds := []*exec.Cmd{
		newSimpleCmd(0, "1", 0),
		newSimpleCmd(0, "2", 0),
		newSimpleCmd(0, "3", 0),
		newSimpleCmd(0, "4", 0),
		newSimpleCmd(0, "5", 0),
	}
	testEnv := newTestEnv(5, cmds)
	require.NoError(t, testEnv.run())

	testEnv.eventHandler.StartedEventSuccess(t)
	testEnv.eventHandler.FinishedEventSuccess(t)
	testEnv.eventHandler.NumEventsForTypeSuccess(t, EventTypeCmdStarted, 5)
	testEnv.eventHandler.NumEventsForTypeSuccess(t, EventTypeCmdFinished, 5)
	require.Equal(t, []string{"1", "2", "3", "4", "5"}, testEnv.stdout.SortedLines(t))
}

func TestError(t *testing.T) {
	cmds := []*exec.Cmd{
		newSimpleCmd(0, "1", 0),
		newSimpleCmd(0, "2", 1),
		newSimpleCmd(0, "3", 0),
		newSimpleCmd(0, "4", 0),
		newSimpleCmd(0, "5", 0),
	}
	testEnv := newTestEnv(5, cmds)
	require.Error(t, testEnv.run())

	testEnv.eventHandler.StartedEventSuccess(t)
	testEnv.eventHandler.FinishedEventError(t)
	testEnv.eventHandler.NumEventsForTypeSuccess(t, EventTypeCmdStarted, 5)
	testEnv.eventHandler.NumEventsForTypeSuccess(t, EventTypeCmdFinished, 4)
	testEnv.eventHandler.NumEventsForTypeError(t, EventTypeCmdFinished, 1)
	require.Equal(t, []string{"1", "2", "3", "4", "5"}, testEnv.stdout.SortedLines(t))
}

func newSimpleCmd(sleepSec int, echoString string, exitCode int) *exec.Cmd {
	return exec.Command(
		"./testdata/bin/simple.sh",
		strconv.Itoa(sleepSec),
		echoString,
		strconv.Itoa(exitCode),
	)
}

type testEnv struct {
	maxConcurrentCmds int
	cmds              []*exec.Cmd
	runner            *runner
	eventHandler      *testEventHandler
	stdout            *testBuffer
	stderr            *testBuffer
}

func newTestEnv(maxConcurrentCmds int, cmds []*exec.Cmd) *testEnv {
	stdout := newConcurrentReadWriter()
	stderr := newConcurrentReadWriter()
	for _, cmd := range cmds {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
	}
	eventHandler := newTestEventHandler()
	return &testEnv{
		maxConcurrentCmds,
		cmds,
		newRunner(
			WithMaxConcurrentCmds(maxConcurrentCmds),
			WithEventHandler(eventHandler.Handle),
		),
		eventHandler,
		stdout,
		stderr,
	}
}

func (e *testEnv) run() error {
	return e.runner.Run(ExecCmds(e.cmds))
}

type testEventHandler struct {
	events []*Event
	lock   sync.RWMutex
}

func newTestEventHandler() *testEventHandler {
	return &testEventHandler{}
}

func (e *testEventHandler) Handle(event *Event) {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.events = append(e.events, event)
}

func (e *testEventHandler) EventsForType(eventType EventType) []*Event {
	e.lock.RLock()
	defer e.lock.RUnlock()
	var eventsForType []*Event
	for _, event := range e.events {
		if event.Type == eventType {
			eventsForType = append(eventsForType, event)
		}
	}
	return eventsForType
}

func (e *testEventHandler) EventsForTypeSuccess(eventType EventType) []*Event {
	eventsForType := e.EventsForType(eventType)
	var eventsForTypeSuccess []*Event
	for _, event := range eventsForType {
		if event.Error == "" {
			eventsForTypeSuccess = append(eventsForTypeSuccess, event)
		}
	}
	return eventsForTypeSuccess
}

func (e *testEventHandler) EventsForTypeError(eventType EventType) []*Event {
	eventsForType := e.EventsForType(eventType)
	var eventsForTypeError []*Event
	for _, event := range eventsForType {
		if event.Error != "" {
			eventsForTypeError = append(eventsForTypeError, event)
		}
	}
	return eventsForTypeError
}

func (e *testEventHandler) NumEventsForType(t *testing.T, eventType EventType, num int) []*Event {
	eventsForType := e.EventsForType(eventType)
	require.Len(t, eventsForType, num)
	return eventsForType
}

func (e *testEventHandler) NumEventsForTypeSuccess(t *testing.T, eventType EventType, num int) []*Event {
	eventsForType := e.EventsForTypeSuccess(eventType)
	require.Len(t, eventsForType, num)
	return eventsForType
}

func (e *testEventHandler) NumEventsForTypeError(t *testing.T, eventType EventType, num int) []*Event {
	eventsForType := e.EventsForTypeError(eventType)
	require.Len(t, eventsForType, num)
	return eventsForType
}

func (e *testEventHandler) OneEventForType(t *testing.T, eventType EventType) *Event {
	return e.NumEventsForType(t, eventType, 1)[0]
}

func (e *testEventHandler) OneEventForTypeSuccess(t *testing.T, eventType EventType) *Event {
	return e.NumEventsForTypeSuccess(t, eventType, 1)[0]
}

func (e *testEventHandler) OneEventForTypeError(t *testing.T, eventType EventType) *Event {
	return e.NumEventsForTypeError(t, eventType, 1)[0]
}

func (e *testEventHandler) StartedEvent(t *testing.T) *Event {
	return e.OneEventForType(t, EventTypeStarted)
}

func (e *testEventHandler) StartedEventSuccess(t *testing.T) *Event {
	return e.OneEventForTypeSuccess(t, EventTypeStarted)
}

func (e *testEventHandler) StartedEventError(t *testing.T) *Event {
	return e.OneEventForTypeSuccess(t, EventTypeStarted)
}

func (e *testEventHandler) FinishedEvent(t *testing.T) *Event {
	return e.OneEventForType(t, EventTypeFinished)
}

func (e *testEventHandler) FinishedEventSuccess(t *testing.T) *Event {
	return e.OneEventForTypeSuccess(t, EventTypeFinished)
}

func (e *testEventHandler) FinishedEventError(t *testing.T) *Event {
	return e.OneEventForTypeError(t, EventTypeFinished)
}

type testBuffer struct {
	buffer *bytes.Buffer
	lock   sync.RWMutex
}

func newConcurrentReadWriter() *testBuffer {
	return &testBuffer{bytes.NewBuffer(nil), sync.RWMutex{}}
}

func (b *testBuffer) Read(p []byte) (int, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.buffer.Read(p)
}

func (b *testBuffer) Write(p []byte) (int, error) {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.buffer.Write(p)
}

func (b *testBuffer) SortedLines(t *testing.T) []string {
	b.lock.RLock()
	defer b.lock.RUnlock()
	return getSortedLines(t, b.buffer)
}

func (b *testBuffer) Lines(t *testing.T) []string {
	b.lock.RLock()
	defer b.lock.RUnlock()
	return getLines(t, b.buffer)
}

func getSortedLines(t *testing.T, reader io.Reader) []string {
	lines := getLines(t, reader)
	sort.Strings(lines)
	return lines
}

func getLines(t *testing.T, reader io.Reader) []string {
	var lines []string
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	require.NoError(t, scanner.Err())
	return lines
}
