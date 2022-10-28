package set

import "github.com/google/uuid"

type UUIDSet map[uuid.UUID]int8

var placeholder = int8(1)

// New creates a new uuidset with the specified ids in it
func New(ids ...uuid.UUID) UUIDSet {
	set := make(UUIDSet, len(ids))
	for _, id := range ids {
		set[id] = placeholder
	}
	return set
}

// FromList converts a list of ids a UUIDSet
func FromList(ids []uuid.UUID) UUIDSet {
	return New(ids...)
}

// FromInterfaceList converts a list of interfaces that are known to be ids into a UUIDSet
func FromInterfaceList(ids []interface{}) UUIDSet {
	set := make(UUIDSet, len(ids))
	for _, id := range ids {
		set[id.(uuid.UUID)] = placeholder
	}
	return set
}

// ToList converts a UUIDSet to a list of ids
func (inputSet UUIDSet) ToList() []uuid.UUID {
	returnList := make([]uuid.UUID, 0, len(inputSet))
	for key := range inputSet {
		returnList = append(returnList, key)
	}
	return returnList
}

// Clone copies a uuid set to a new uuid set
func (s UUIDSet) Clone() UUIDSet {
	returnSet := make(UUIDSet, len(s))
	for key, value := range s {
		returnSet[key] = value
	}
	return returnSet
}

// Intersect returns a new UUIDSet with the intersection of all the elements in both sets
func (s1 UUIDSet) Intersect(s2 UUIDSet) UUIDSet {
	return setOperation(s1, s2, true)
}

// Minus returns a new UUIDSet with the
func (s1 UUIDSet) Minus(s2 UUIDSet) UUIDSet {
	return setOperation(s1, s2, false)
}

// setOperation is a helper method to either intersect or subtract sets
func setOperation(s1, s2 UUIDSet, wantElemsInSet2 bool) UUIDSet {
	resultSet := make(UUIDSet)
	for key := range s1 {
		if _, ok := s2[key]; ok == wantElemsInSet2 {
			resultSet[key] = placeholder
		}
	}
	return resultSet
}

// AddSet adds all the elements in a uuid set to the operand set.
func (s UUIDSet) AddSet(newValues UUIDSet) {
	for newValue := range newValues {
		s[newValue] = placeholder
	}
}

// AddAll adds all the elements in a uuid slice to the operand set.
func (s UUIDSet) AddAll(newValues []uuid.UUID) {
	for _, newValue := range newValues {
		s[newValue] = placeholder
	}
}

// Equals returns true if two uuid sets have exactly the same elements
func (s1 UUIDSet) Equals(s2 UUIDSet) bool {
	if len(s1) != len(s2) {
		return false
	}
	for key := range s1 {
		if _, ok := s2[key]; !ok {
			return false
		}
	}
	return true
}

// Add adds an element to a uuid set
func (s UUIDSet) Add(id uuid.UUID) {
	s[id] = placeholder
}

// Delete removes an element from a uuid set
func (s UUIDSet) Remove(id uuid.UUID) {
	delete(s, id)
}

// Contains returns true if a uuidset contains the specified uuid
func (s UUIDSet) Contains(id uuid.UUID) bool {
	_, ok := s[id]
	return ok
}

// Partition takes in two uuid slices and returns a tuple with (ids only in the first set,
// ids in both sets, ids only in the second set). It is a utility function that uses the
// set implmentation
func Partition(s1, s2 []uuid.UUID) (only1 []uuid.UUID, both []uuid.UUID, only2 []uuid.UUID) {
	set1 := FromList(s1)
	set2 := FromList(s2)
	return set1.Minus(set2).ToList(), set1.Intersect(set2).ToList(), set2.Minus(set1).ToList()
}
