package api

import "testing"

func TestGetServerName(t *testing.T) {

	s, err := getServerName("jphnd1-21wase0.devel.mon.ntppool.dev")
	if err != nil {
		t.Log(err)
		t.Fail()
	}
	if s != "https://api.devel.mon.ntppool.dev" {
		t.Logf("got unexpected development server URL: %s", s)
		t.Fail()

	}

}
