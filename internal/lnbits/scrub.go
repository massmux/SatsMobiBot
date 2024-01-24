package lnbits

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
)

type Scrub struct {
	ApiKey          string
	LnbitsPublicURL string
}

func (s Scrub) WalletID() string {
	url := s.LnbitsPublicURL + "api/v1/wallet"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.ApiKey)

	client := &http.Client{}
	resp, _ := client.Do(req)
	body, _ := ioutil.ReadAll(resp.Body)

	var result map[string]interface{}
	json.Unmarshal([]byte(body), &result)

	return result["id"].(string)
}

func (s *Scrub) ScrubCreate(walletID string, lnAddress string, scrubName string) map[string]interface{} {
	scrub := s.ScrubExists(scrubName)
	if scrub != nil {
		s.ScrubDelete(scrub["id"].(string))
	}
	url := s.LnbitsPublicURL + "scrub/api/v1/links"
	payload := map[string]string{
		"wallet":       walletID,
		"description":  scrubName,
		"payoraddress": lnAddress,
	}
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.ApiKey)
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	return result
}

func (s *Scrub) scrubList() []map[string]interface{} {
	url := s.LnbitsPublicURL + "scrub/api/v1/links"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.ApiKey)
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	var results []map[string]interface{}
	json.Unmarshal(body, &results)
	return results
}

func (s *Scrub) ScrubDelete(scrubID string) map[string]interface{} {
	url := s.LnbitsPublicURL + "scrub/api/v1/links/" + scrubID
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.ApiKey)
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	return result
}

func (s *Scrub) ScrubExists(scrubName string) map[string]interface{} {
	scrubs := s.scrubList()
	//res := make(map[string]interface{})
	for _, i := range scrubs {
		if i["description"] == scrubName {
			return i
		}
	}
	return nil
}
