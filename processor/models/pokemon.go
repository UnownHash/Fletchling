package models

import "fmt"

type PokemonKey struct {
	PokemonId int `json:"pokemon_id"`
	FormId    int `json:"form_id"`
}

// Since we use this in map keys and I'm lazy and don't
// feel like converting the maps to return from API for
// stats.. This will marshal into a string.
func (k PokemonKey) MarshalText() ([]byte, error) {
	return []byte(k.String()), nil
}

func (k PokemonKey) String() string {
	return fmt.Sprintf("%d:%d", k.PokemonId, k.FormId)
}

type Pokemon struct {
	PokemonId    int
	FormId       int
	SpawnpointId uint64
	Lat          float64
	Lon          float64
}

func (pokemon Pokemon) Key() PokemonKey {
	return PokemonKey{PokemonId: pokemon.PokemonId, FormId: pokemon.FormId}
}
