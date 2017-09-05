package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"
)

const VERSION = "2.0"

type API struct {
	url string
}

type ServerList struct {
	Servers []string `json:"servers"`
}

var client http.Client

func init() {
	client = http.Client{
		Timeout: 15 * time.Second,
	}
}

func NewAPI(apiURL, apiKey string) (*API, error) {
	apiurl, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("Invalid url %q: %s", apiURL, err)
	}

	urlq := apiurl.Query()
	urlq.Set("api_key", apiKey)
	apiurl.RawQuery = urlq.Encode()

	log.Printf("API: %s", apiurl.String())

	return &API{
		url: apiurl.String(),
	}, nil

}

func (api *API) newRequest() (*http.Request, error) {
	req, err := http.NewRequest("GET", api.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ntppool-monitor/"+VERSION)
	return req, nil
}

func (api *API) GetServerList() ([]net.IP, error) {

	req, err := api.newRequest()
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected API status code: %d", resp.StatusCode)
	}

	serverlist := &ServerList{}

	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&serverlist)
	if err != nil {
		return nil, fmt.Errorf("json %s", err)
	}

	servers := []net.IP{}
	for _, s := range serverlist.Servers {
		sip := net.ParseIP(s)
		if sip == nil {
			log.Printf("invalid IP %q", s)
			continue
		}
		servers = append(servers, sip)
	}
	return servers, nil
}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func (api *API) PostStatuses(statuses []*ServerStatus) error {

	log.Printf("Posting statuses!")

	req, err := api.newRequest()
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	feedback := MonitorFeedback{
		Version: 1,
		Servers: statuses,
	}

	b, err := json.Marshal(&feedback)
	if err != nil {
		return err
	}

	log.Printf("Feedback %s\n", b)

	r := bytes.NewBuffer(b)
	req.Method = "POST"
	req.Body = nopCloser{r}

	// pretty.Print(req)
	// log.Printf("post request: %s", req)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected API status code: %d", resp.StatusCode)
	}

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.Printf("Response: %s", content)

	return nil

}
