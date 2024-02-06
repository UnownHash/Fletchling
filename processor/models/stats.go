package models

import "time"

type NestTimePeriodSummary struct {
	Nest      *Nest
	StartTime time.Time
	EndTime   time.Time
	// actual sum of durations of the periods. may be lower than
	// (EndTime - StartTime) if this is being used as totals
	// for all time periods, and a time period was dropped.
	Duration               time.Duration
	PokemonCountsAndTotals NestPokemonCountsAndTotals
}

type NestPokemonCountAndTotal struct {
	Rank int

	PokemonKey PokemonKey
	// Count of PokemonKey in this nest
	Count uint64
	// Total of all pokemon in this nest
	Total uint64
	// Count of PokemonKey globally
	Global uint64
	// Total of all pokemon globally
	GlobalTotal uint64
}

func (np *NestPokemonCountAndTotal) NestPct() float64 {
	if np.Total == 0 {
		return 0
	}
	return 100 * float64(np.Count) / float64(np.Total)
}

func (np *NestPokemonCountAndTotal) GlobalPct() float64 {
	if np.GlobalTotal == 0 {
		return 0
	}
	return 100 * float64(np.Global) / float64(np.GlobalTotal)
}

type NestPokemonCountsAndTotals []NestPokemonCountAndTotal

func (np NestPokemonCountsAndTotals) Len() int {
	return len(np)
}

func (np NestPokemonCountsAndTotals) Swap(i, j int) {
	np[i], np[j] = np[j], np[i]
	np[i].Rank, np[j].Rank = i+1, j+1
}

// sort by Count DESC, Global ASC
func (np NestPokemonCountsAndTotals) Less(i, j int) bool {
	iEntry, jEntry := np[i], np[j]

	if iEntry.Count == jEntry.Count {
		return iEntry.Global < jEntry.Global
	}
	return iEntry.Count > jEntry.Count
}
