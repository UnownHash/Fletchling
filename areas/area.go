package areas

import (
	"strings"
)

type AreaName struct {
	Parent   string
	Name     string
	FullName string
}

func (an AreaName) String() string {
	return an.FullName
}

func (an AreaName) Matches(wildcardedAreas []AreaName) bool {
	return AreaNameMatches(an, wildcardedAreas)
}

func NewAreaName(parent, name string) AreaName {
	var fullName string

	if parent != "" && parent != "*" {
		if name == "" || parent == name {
			fullName = parent
		} else {
			fullName = parent + "/" + name
		}
	} else if name != "" {
		fullName = name
	} else {
		fullName = "world"
	}
	return AreaName{
		Parent:   parent,
		Name:     name,
		FullName: fullName,
	}
}

func AreaStringToAreaName(area string) AreaName {
	splitted := strings.Split(area, "/") // "London/*", "London/Chelsea", "Chelsea"
	if len(splitted) == 2 {
		return NewAreaName(splitted[0], splitted[1])
	}
	return NewAreaName("*", area)
}

func AreaStringsToAreaNames(areas []string) []AreaName {
	areaNames := make([]AreaName, len(areas))
	for idx, area := range areas {
		areaNames[idx] = AreaStringToAreaName(area)
	}
	return areaNames
}

func AreaNameMatches(area AreaName, wildcardedAreas []AreaName) bool {
	for _, wildcardedArea := range wildcardedAreas {
		if wildcardedArea.Name == "*" {
			if wildcardedArea.Parent == area.Parent {
				return true
			}
		} else if wildcardedArea.Parent == "*" {
			if wildcardedArea.Name == area.Name {
				return true
			}
		} else {
			if wildcardedArea.Parent == area.Parent && wildcardedArea.Name == area.Name {
				return true
			}
		}
	}
	return false
}
