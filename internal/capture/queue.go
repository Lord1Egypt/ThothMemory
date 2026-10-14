package capture

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"
)

// entry is one write that could not be delivered. It keeps its operation ID,
// so replaying it can only return the original result, never a second record.
type entry struct {
	Tool    string         `json:"tool"`
	Args    map[string]any `json:"args"`
	Created time.Time      `json:"created"`
}

type rejected struct {
	entry
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

// queue is a small, bounded, owner-only JSON-lines file. It exists only for
// writes that failed because the memory was unreachable.
type queue struct{ d Dir }

func (q queue) file() string    { return q.d.path("pending.jsonl") }
func (q queue) rejects() string { return q.d.path("rejected.jsonl") }
func (q queue) lockDir() string { return q.d.path("pending.lock") }

// withLock serializes hook processes. A lock older than 30 s belongs to a
// process that died and is taken over.
func (q queue) withLock(fn func() error) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		e := os.Mkdir(q.lockDir(), 0700)
		if e == nil {
			defer os.Remove(q.lockDir())
			return fn()
		}
		if !os.IsExist(e) {
			return e
		}
		if st, se := os.Stat(q.lockDir()); se == nil && time.Since(st.ModTime()) > 30*time.Second {
			_ = os.Remove(q.lockDir())
			continue
		}
		if time.Now().After(deadline) {
			return errors.New("pending queue is locked by another process")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (q queue) read() ([]entry, error) {
	b, e := os.ReadFile(q.file())
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var out []entry
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var en entry
		if json.Unmarshal(sc.Bytes(), &en) == nil && en.Tool != "" {
			out = append(out, en)
		}
	}
	return out, sc.Err()
}

func (q queue) store(list []entry) error {
	if len(list) == 0 {
		if e := os.Remove(q.file()); e != nil && !os.IsNotExist(e) {
			return e
		}
		return nil
	}
	var buf bytes.Buffer
	for _, en := range list {
		b, _ := json.Marshal(en)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return writeAtomic(q.file(), buf.Bytes())
}

func (q queue) push(en entry) error {
	return q.withLock(func() error {
		list, e := q.read()
		if e != nil {
			return e
		}
		b, _ := json.Marshal(en)
		size := len(b) + 1
		for _, x := range list {
			xb, _ := json.Marshal(x)
			size += len(xb) + 1
		}
		if len(list) >= maxQueue || size > maxQueueBytes {
			return errors.New("pending queue is full; run the pending command after memory is reachable")
		}
		return q.store(append(list, en))
	})
}

// drain replays queued writes in order. It stops at the first transient
// failure (memory still unreachable). A write the memory refuses is moved to
// rejected.jsonl with the reason, never silently dropped or retried forever.
func (q queue) drain(ctx context.Context, c Caller) (sent, left int, err error) {
	err = q.withLock(func() error {
		list, e := q.read()
		if e != nil || len(list) == 0 {
			return e
		}
		keep := list
		for i, en := range list {
			_, ce := c.Call(ctx, en.Tool, en.Args)
			if ce == nil {
				sent++
				keep = list[i+1:]
				continue
			}
			if Transient(ce) {
				keep = list[i:]
				break
			}
			f, oe := os.OpenFile(q.rejects(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			if oe != nil {
				keep = list[i:]
				return oe
			}
			b, _ := json.Marshal(rejected{en, ce.Error(), time.Now().UTC()})
			_, we := f.Write(append(b, '\n'))
			f.Close()
			if we != nil {
				keep = list[i:]
				return we
			}
			keep = list[i+1:]
		}
		left = len(keep)
		if sent == 0 && left == len(list) {
			return nil
		}
		return q.store(keep)
	})
	return
}

// Pending reports queue size and rejected writes for the owner.
func Pending(d Dir) (pending, rejectedCount int) {
	q := queue{d}
	list, _ := q.read()
	if b, e := os.ReadFile(q.rejects()); e == nil {
		rejectedCount = bytes.Count(b, []byte("\n"))
	}
	return len(list), rejectedCount
}

// Drain replays the queue now; it is the owner-facing retry command.
func Drain(ctx context.Context, c Caller, d Dir) (sent, left int, err error) {
	return queue{d}.drain(ctx, c)
}
