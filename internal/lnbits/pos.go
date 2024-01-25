package lnbits

import (
	"bytes"
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
)

type Tpos struct {
	ApiKey          string
	LnbitsPublicUrl string
}

type PosData struct {
	Name            string      `json:"name"`
	Currency        string      `json:"currency"`
	TipOptions      string      `json:"tip_options"`
	TipWallet       string      `json:"tip_wallet"`
	Withdrawlimit   interface{} `json:"withdrawlimit"`
	Withdrawpin     interface{} `json:"withdrawpin"`
	Withdrawamt     int         `json:"withdrawamt"`
	Withdrawtime    int         `json:"withdrawtime"`
	Withdrawbtwn    int         `json:"withdrawbtwn"`
	Withdrawpremium interface{} `json:"withdrawpremium"`
	Items           string      `json:"items"`
}

type Pos struct {
	ID   string
	Name string
}

func (t *Tpos) PosCreate(posName string, posCurrency string) string {
	// if pos with that name exists, then return the ID
	if posID := t.PosExists(posName); posID != "" {
		//t.PosDelete(posID)
		return posID
	}

	url := t.LnbitsPublicUrl + "tpos/api/v1/tposs"
	data := PosData{
		Name:         posName,
		Currency:     posCurrency,
		TipOptions:   "[]",
		Withdrawamt:  0,
		Withdrawtime: 0,
		Withdrawbtwn: 10,
		Items:        "[]",
	}

	jsonValue, err := json.Marshal(data)
	if err != nil {
		log.Fatalf("Error occurred during marshaling data: %v", err)
	}

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", t.ApiKey)

	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	//var result map[string]interface{}
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)

	myid := result["id"]
	return myid
}

func (t *Tpos) PosList() []Pos {
	req, _ := http.NewRequest("GET", t.LnbitsPublicUrl+"tpos/api/v1/tposs", nil)
	req.Header.Set("Content-type", "application/json")
	req.Header.Set("X-Api-Key", t.ApiKey)

	client := &http.Client{}
	resp, _ := client.Do(req)

	body, _ := ioutil.ReadAll(resp.Body)

	var pos []Pos
	_ = json.Unmarshal(body, &pos)

	return pos
}

func (t *Tpos) PosDelete(posID string) map[string]interface{} {
	url := t.LnbitsPublicUrl + "tpos/api/v1/tposs/" + posID

	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", t.ApiKey)

	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	return result
}

func (t *Tpos) PosExists(posName string) string {
	posList := t.PosList()

	for _, pos := range posList {
		if pos.Name == posName {
			return pos.ID
		}
	}

	return ""
}
