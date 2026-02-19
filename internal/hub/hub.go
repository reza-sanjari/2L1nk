package hub

type Hub struct{}

func New() *Hub {
	return &Hub{}
}

func (h *Hub) Status() (string, error) {
	return "OK", nil
}
