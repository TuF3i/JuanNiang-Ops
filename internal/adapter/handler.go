package adapter

func (r *Runtime) Rest() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.restSignal <- struct{}{}
}

func (r *Runtime) OnRest() {

}
