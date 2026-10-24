package planner

// Dependencies between tasks: a task can wait on others (finish-to-start) —
// it starts after they're due. When one's end moves, what waits on it moves
// as many days (keeping its length), never starting before everything it
// waits on has ended; and so on down the chain.

// Dated is what scheduling needs of a task.
type Dated struct {
	ID      string
	Start   *string // YYYY-MM-DD
	Due     *string
	WaitsOn []string
	Done    bool
}

func (d Dated) from() (string, bool) {
	if d.Start != nil {
		return *d.Start, true
	}
	if d.Due != nil {
		return *d.Due, true
	}
	return "", false
}

func (d Dated) end() (string, bool) {
	if d.Due != nil {
		return *d.Due, true
	}
	if d.Start != nil {
		return *d.Start, true
	}
	return "", false
}

// shift moves both of a task's dates by n days.
func (d *Dated) shift(n int) {
	for _, p := range []**string{&d.Start, &d.Due} {
		if *p != nil {
			if s, ok := ShiftDate(**p, n); ok {
				*p = &s
			}
		}
	}
}

// Cascade moves what waits on task id now that its end moved by delta days
// (tasks holds the workspace's tasks, id's new dates included). It returns
// the tasks it moved, with their new dates. Done and undated tasks stay.
func Cascade(tasks []Dated, id string, delta int) []Dated {
	if delta == 0 {
		return nil
	}
	byID := make(map[string]*Dated, len(tasks))
	waiting := map[string][]string{} // id → the tasks waiting on it
	for i := range tasks {
		t := &tasks[i]
		byID[t.ID] = t
		for _, p := range t.WaitsOn {
			waiting[p] = append(waiting[p], t.ID)
		}
	}
	moved := map[string]bool{}
	var order []string
	type step struct {
		id    string
		delta int
	}
	queue := []step{{id, delta}}
	for n := 0; len(queue) > 0 && n < 10000; n++ {
		cur := queue[0]
		queue = queue[1:]
		for _, sid := range waiting[cur.id] {
			s := byID[sid]
			from, ok := s.from()
			if !ok || s.Done {
				continue
			}
			want, _ := ShiftDate(from, cur.delta)
			// Not before the day after the latest end of what it waits on.
			for _, pid := range s.WaitsOn {
				p := byID[pid]
				if p == nil || p.Done {
					continue
				}
				if e, ok := p.end(); ok {
					if after, _ := ShiftDate(e, 1); after > want {
						want = after
					}
				}
			}
			d := DaysBetween(from, want)
			if d == 0 {
				continue
			}
			s.shift(d)
			if !moved[sid] {
				moved[sid] = true
				order = append(order, sid)
			}
			queue = append(queue, step{sid, d})
		}
	}
	out := make([]Dated, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out
}

// Reaches reports whether, following WaitsOn from the tasks in start, one
// arrives at target — i.e. making target wait on them would be a loop.
func Reaches(waitsOn map[string][]string, start []string, target string) bool {
	seen := map[string]bool{}
	stack := append([]string(nil), start...)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if id == target {
			return true
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		stack = append(stack, waitsOn[id]...)
	}
	return false
}
