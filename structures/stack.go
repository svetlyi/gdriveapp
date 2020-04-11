package structures

import "errors"

type StringStack struct {
	elements []string
}

func (s *StringStack) Push(el string) {
	s.elements = append(s.elements, el)
}

func (s *StringStack) Pop() error {
	if len(s.elements) > 0 {
		s.elements = s.elements[:len(s.elements)-1]
	} else {
		return errors.New("no elements")
	}
	return nil
}

// PopTimes pops [times] times. Basically it removes [times] elements
// from the top.
func (s *StringStack) PopTimes(times int) (err error) {
	for times > 0 && nil == err {
		err = s.Pop()
		times--
	}
	return
}

func (s *StringStack) Front() (string, error) {
	if len(s.elements) > 0 {
		return s.elements[len(s.elements)-1], nil
	} else {
		return "", errors.New("no elements")
	}
}
func (s *StringStack) Len() int {
	return len(s.elements)
}
