package registry

import "slices"

type Registry[E any] struct {
	keys    map[string]int32
	values  []E
	indices map[*E]int32
	tags    map[string][]*E
}

func NewRegistry[E any]() Registry[E] {
	return Registry[E]{
		keys:    make(map[string]int32),
		values:  make([]E, 0, 256),
		indices: make(map[*E]int32),
		tags:    make(map[string][]*E),
	}
}

func (r *Registry[E]) Clear() {
	r.keys = make(map[string]int32)
	r.values = r.values[:0]
	r.indices = make(map[*E]int32)
	r.tags = make(map[string][]*E)
}

func (r *Registry[E]) Get(key string) (int32, *E) {
	id, ok := r.keys[key]
	if !ok {
		return -1, nil
	}
	return id, &r.values[id]
}

func (r *Registry[E]) GetByID(id int32) *E {
	if id >= 0 && id < int32(len(r.values)) {
		return &r.values[id]
	}
	return nil
}

// Keys returns every registered key, in registration order (i.e. sorted by
// id). It is primarily a debugging/introspection helper: callers that already
// know the id they want should use GetByID, which is O(1).
func (r *Registry[E]) Keys() []string {
	out := make([]string, 0, len(r.values))
	for id := range r.values {
		out = append(out, r.idToKey(int32(id)))
	}
	return out
}

func (r *Registry[E]) idToKey(id int32) string {
	for k, v := range r.keys {
		if v == id {
			return k
		}
	}
	return ""
}

func (r *Registry[E]) Put(key string, data E) (id int32, val *E) {
	id = int32(len(r.values))
	r.keys[key] = id
	r.values = append(r.values, data)
	val = &r.values[id]
	r.indices[val] = id
	return
}

// Tags

func (r *Registry[E]) Tag(tag string) []*E {
	return slices.Clone(r.tags[tag])
}

func (r *Registry[E]) ClearTags() {
	r.tags = make(map[string][]*E)
}

// func (r *Registry[E]) BindTags(tag string, ids []int32) error {
// 	values := make([]*E, len(ids))
// 	for i, id := range ids {
// 		if id < 0 || id >= int32(len(r.values)) {
// 			return errors.New("invalid id: " + strconv.Itoa(int(id)))
// 		}
// 		values[i] = &r.values[id]
// 	}
// 	r.tags[tag] = values
// 	return nil
// }
