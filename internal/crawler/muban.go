package crawler

import "github.com/dop251/goja"

// mubanObject stores arbitrary nested muban properties while exposing them to JS.
type mubanObject struct {
	vm       *goja.Runtime
	values   map[string]any
	children map[string]*mubanObject
}

func newMubanObject(vm *goja.Runtime) *mubanObject {
	return &mubanObject{vm: vm, values: make(map[string]any), children: make(map[string]*mubanObject)}
}

func (m *mubanObject) Get(key string) goja.Value {
	if child, ok := m.children[key]; ok {
		return m.vm.NewDynamicObject(child)
	}
	if value, ok := m.values[key]; ok {
		return m.vm.ToValue(value)
	}
	// Auto-vivify intermediate properties so assignments such as muban.a.b.c work.
	child := newMubanObject(m.vm)
	m.children[key] = child
	return m.vm.NewDynamicObject(child)
}

func (m *mubanObject) Set(key string, value goja.Value) bool {
	delete(m.children, key)
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		m.values[key] = nil
		return true
	}
	m.values[key] = value.Export()
	return true
}

func (m *mubanObject) Has(key string) bool {
	_, inValues := m.values[key]
	_, inChildren := m.children[key]
	return inValues || inChildren
}

func (m *mubanObject) Delete(key string) bool {
	delete(m.values, key)
	delete(m.children, key)
	return true
}

func (m *mubanObject) Keys() []string {
	keys := make([]string, 0, len(m.values)+len(m.children))
	for key := range m.values {
		keys = append(keys, key)
	}
	for key := range m.children {
		if _, ok := m.values[key]; !ok {
			keys = append(keys, key)
		}
	}
	return keys
}

func installMuban(e *Engine) {
	_ = e.vm.Set("muban", e.vm.NewDynamicObject(newMubanObject(e.vm)))
}

func (e *Engine) readMuban() map[string]any {
	out := make(map[string]any)
	if e == nil || e.vm == nil {
		return out
	}
	value := e.vm.Get("muban")
	root, ok := value.Export().(*mubanObject)
	if !ok {
		return out
	}
	var flatten func(*mubanObject, string)
	flatten = func(obj *mubanObject, prefix string) {
		for key, val := range obj.values {
			name := key
			if prefix != "" {
				name = prefix + "." + key
			}
			out[name] = val
		}
		for key, child := range obj.children {
			name := key
			if prefix != "" {
				name = prefix + "." + key
			}
			flatten(child, name)
		}
	}
	flatten(root, "")
	return out
}
