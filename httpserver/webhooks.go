package httpserver

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/guregu/null.v4"

	"github.com/UnownHash/Fletchling/processor/models"
)

type decoderFn func([]byte, *WebhookMessage) error

var decoderTypes = map[string]decoderFn{
	"pokemon": func(b []byte, msg *WebhookMessage) error {
		return json.Unmarshal(b, &msg.Pokemon)
	},
}

type WebhookMessage struct {
	Type    string `json:"type"`
	Pokemon *PokemonWebhook
}

func (msg *WebhookMessage) UnmarshalJSON(b []byte) error {
	var hook struct {
		Type    string          `json:"type"`
		Message json.RawMessage `json:"message"`
	}

	err := json.Unmarshal(b, &hook)
	if err != nil {
		return err
	}

	decoder := decoderTypes[hook.Type]
	if decoder == nil {
		log.Printf("unknown hook type: %s", hook.Type)
		// silently drop unknown webhook types
		return nil
	}
	return decoder([]byte(hook.Message), msg)
}

type PokemonWebhook struct {
	SpawnpointId          string          `json:"spawnpoint_id"`
	PokestopId            string          `json:"pokestop_id"`
	EncounterId           string          `json:"encounter_id"`
	PokemonId             int             `json:"pokemon_id"`
	Latitude              float64         `json:"latitude"`
	Longitude             float64         `json:"longitude"`
	DisappearTime         int64           `json:"disappear_time"`
	DisappearTimeVerified bool            `json:"disappear_time_verified"`
	FirstSeen             int64           `json:"first_seen"`
	LastModifiedTime      null.Int        `json:"last_modified_time"`
	Gender                null.Int        `json:"gender"`
	Cp                    null.Int        `json:"cp"`
	Form                  null.Int        `json:"form"`
	Costume               null.Int        `json:"costume"`
	IndividualAttack      null.Int        `json:"individual_attack"`
	IndividualDefense     null.Int        `json:"individual_defense"`
	IndividualStamina     null.Int        `json:"individual_stamina"`
	PokemonLevel          null.Int        `json:"pokemon_level"`
	Move1                 null.Int        `json:"move_1"`
	Move2                 null.Int        `json:"move_2"`
	Weight                null.Float      `json:"weight"`
	Size                  null.Int        `json:"size"`
	Height                null.Float      `json:"height"`
	Weather               null.Int        `json:"weather"`
	Capture1              float64         `json:"capture_1"`
	Capture2              float64         `json:"capture_2"`
	Capture3              float64         `json:"capture_3"`
	Shiny                 null.Bool       `json:"shiny"`
	Username              null.String     `json:"username"`
	DisplayPokemonId      null.Int        `json:"display_pokemon_id"`
	IsEvent               int8            `json:"is_event"`
	SeenType              null.String     `json:"seen_type"`
	PVP                   json.RawMessage `json:"pvp"`
}

func (wh *PokemonWebhook) SpawnpointIdAsInt() (uint64, error) {
	return strconv.ParseUint(wh.SpawnpointId, 16, 64)
}

func (wh *PokemonWebhook) EncounterIdAsInt() (uint64, error) {
	return strconv.ParseUint(wh.EncounterId, 0, 64)
}

func (srv *HTTPServer) processMessages(msgs []WebhookMessage) {
	var numProcessed uint64

	now := time.Now()

	for _, msg := range msgs {
		pokemon := msg.Pokemon
		if pokemon == nil {
			srv.logger.Warnf("ignoring webhook for type '%s': please only send me pokemon!", msg.Type)
			continue
		}

		if pokemon.PokemonId <= 0 {
			srv.logger.Warnf("ignoring pokemon webhook with bad pokemon id (%#v)", *pokemon)
			continue
		}

		spawnpointId, err := pokemon.SpawnpointIdAsInt()
		if err != nil {
			if pokemon.SpawnpointId != "None" && pokemon.SpawnpointId != "" {
				srv.logger.Warnf("ignoring pokemon webhook with no or bad spawnpoint id: %s (%#v)", err, *pokemon)
				continue
			}
			// lured pokemon, likely. Will match by area.
			spawnpointId = 0
		}

		if !pokemon.IndividualAttack.Valid {
			// only look at encounters.
			continue
		}

		npPokemon := models.Pokemon{
			PokemonId:    pokemon.PokemonId,
			FormId:       int(pokemon.Form.ValueOrZero()),
			SpawnpointId: spawnpointId,
			Lat:          pokemon.Latitude,
			Lon:          pokemon.Longitude,
		}

		srv.nestProcessorManager.ProcessPokemon(&npPokemon)
		numProcessed++
	}

	srv.statsCollector.AddPokemonProcessed(numProcessed)
	srv.logger.Debugf("processed %d pokemon from single webhook in %s", numProcessed, time.Now().Sub(now).Truncate(time.Millisecond))
}

func (srv *HTTPServer) handleWebhook(c *gin.Context) {
	var msgs []WebhookMessage

	decoder := json.NewDecoder(c.Request.Body)
	err := decoder.Decode(&msgs)
	if err != nil {
		srv.logger.Warnf("received unprocessable webhook: %s", err)
		// Bad format? I guess treat as success so caller doesn't
		// retry. Tho Golbat doesn't retry at all, anyway.
		c.Status(http.StatusOK)
		return
	}

	go srv.processMessages(msgs)

	c.Status(http.StatusOK)
}
