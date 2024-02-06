package processor

import (
	"github.com/UnownHash/Fletchling/processor/models"
)

type PokemonCountAndTotal struct {
	Rank       int
	PokemonKey models.PokemonKey
	Count      uint64
	Total      uint64
}

type PokemonCountAndTotals []PokemonCountAndTotal

func (pc PokemonCountAndTotals) Len() int {
	return len(pc)
}

func (pc PokemonCountAndTotals) Swap(i, j int) {
	pc[i], pc[j] = pc[j], pc[i]
	pc[i].Rank, pc[j].Rank = i+1, j+1
}

// sort by Count DESC, dex ASC, form ASC
func (pc PokemonCountAndTotals) Less(i, j int) bool {
	iEntry, jEntry := pc[i], pc[j]

	if iEntry.Count == jEntry.Count {
		if iEntry.PokemonKey.PokemonId == jEntry.PokemonKey.PokemonId {
			return iEntry.PokemonKey.FormId < jEntry.PokemonKey.FormId
		}
		return iEntry.PokemonKey.PokemonId < jEntry.PokemonKey.PokemonId
	}
	return iEntry.Count > jEntry.Count
}
