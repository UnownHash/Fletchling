package geo

import "strings"

type AreaName struct {
	Parent string
	Name   string
}

func AreaStringToAreaName(area string) AreaName {
	splitted := strings.Split(area, "/") // "London/*", "London/Chelsea", "Chelsea"
	if len(splitted) == 2 {
		return AreaName{Parent: splitted[0], Name: splitted[1]}
	}
	return AreaName{Parent: "*", Name: area}

}

func AreaStringsToAreaNames(areas []string) []AreaName {
	areaNames := make([]AreaName, len(areas))
	for idx, area := range areas {
		areaNames[idx] = AreaStringToAreaName(area)
	}
	return areaNames
}

func AreaMatchWithWildcards(area AreaName, areasToMatch []AreaName) bool {
	for _, toMatchArea := range areasToMatch {
		if toMatchArea.Name == "*" {
			if toMatchArea.Parent == area.Parent {
				return true
			}
		} else if toMatchArea.Parent == "*" {
			if toMatchArea.Name == area.Name {
				return true
			}
		} else {
			if toMatchArea.Parent == area.Parent && toMatchArea.Name == area.Name {
				return true
			}
		}
	}
	return false
}
