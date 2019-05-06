package service

import "testing"

func TestAddPath(t *testing.T) {
	root := node{}
	urlParams := []string{"version", "stationId"}

	AssertNoError(t, root.addPath("", "$version.stations.$stationId", urlParams, "model", ""))
	AssertNoError(t, root.addPath("station", "$version.stations.$stationId.station", urlParams, "model", ""))
	AssertNoError(t, root.addPath("station.transfers", "$version.stations.$stationId.station.transfers", urlParams, "model", ""))
	AssertNoError(t, root.addPath("station.transfers.transfer", "$version.stations.$stationId.station.transfers.transfer", urlParams, "collection", ""))
	AssertNoError(t, root.addPath("station.transfers.transfer.$transferId", "$version.stations.$stationId.station.transfers.transfer.$transferId", urlParams, "model", "id"))
}

func AssertNoError(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("Error: %s", err)
	}
}
