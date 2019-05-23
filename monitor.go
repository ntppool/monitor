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

type Config struct {
	Samples int    `json:"samples"`
	IP      net.IP `json:"ip"`
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

func (api *API) newRequest(path string) (*http.Request, error) {
	switch {
	case len(path) > 0:
		u, err := url.Parse(api.url)
		if err != nil {
			return nil, err
		}
		u.Path = u.Path + "/" + path
		path = u.String()
	default:
		path = api.url
	}

	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ntppool-monitor/"+VERSION)
	return req, nil
}

func (api *API) GetConfig() (*Config, error) {
	resp := struct{ Config *Config }{}
	err := api.getAPI("config", &resp)
	if resp.Config == nil {
		return nil, fmt.Errorf("empty configuration")
	}
	return resp.Config, err
}

func (api *API) GetServerList() (*ServerList, error) {
	serverlist := &ServerList{}
	err := api.getAPI("", serverlist)
	return serverlist, err
}

func (api *API) getAPI(path string, val interface{}) error {

	req, err := api.newRequest(path)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected API status code: %d", resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(val)
	if err != nil {
		return fmt.Errorf("json %s", err)
	}

	return nil
}

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func (api *API) PostStatuses(statuses []*ServerStatus) error {

	log.Printf("Posting statuses!")

	req, err := api.newRequest("")
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	feedback := MonitorFeedback{
		Version: 1,
		Servers: statuses,
	}

	b, err := json.MarshalIndent(&feedback, "", "  ")
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
