package main

import (
	"testing"
	"fmt"
	"strings"
)


// use channel twitch.tv/reckful for testing because he has a legacy account where vods aren't deleted
const vodString string = "187938112"
const vodInt int = 187938112

func TestTokenAPILink(t *testing.T) {
	exampleSig := "7ce3d0ca2c65dd66c7da72c43f4ce72cfcd98a72"
	tokenAPILink := fmt.Sprintf("http://api.twitch.tv/api/vods/%v/access_token?&client_id=aokchnui2n8q38g0vezl9hq6htzy4c", vodInt)
	sig, _, err := accessTokenAPI(tokenAPILink)
	// Testing for length of sig because the sig from twitch is random
	// Havent come up with a meaningful test for token
	if err != nil || len(sig) != len(exampleSig) {
		t.Errorf("Error in accessTokenAPI")
	}
}

func TestAccessUsherAPI(t *testing.T) {
	tokenAPILink := fmt.Sprintf("http://api.twitch.tv/api/vods/%v/access_token?&client_id=aokchnui2n8q38g0vezl9hq6htzy4c", vodInt)
	sig, token, _ := accessTokenAPI(tokenAPILink)

	usherAPILink := fmt.Sprintf("http://usher.twitch.tv/vod/%v?nauthsig=%v&nauth=%v&allow_source=true", vodInt, sig, token)
	edgecastURLmap, err := accessUsherAPI(usherAPILink)

	m3u8Link, _ := edgecastURLmap["chunked"]

	edgecastBaseURL := m3u8Link
	edgecastBaseURL = edgecastBaseURL[0 : strings.Index(edgecastBaseURL, edgecastLinkBaseEnd)]

	//Only checking the end because both
	//http://fastly.vod.hls.ttvnw.net/903cba256ea3055674be_reckful_26660278144_734937575/chunked/
	//http://vod142-ttvnw.akamaized.net/903cba256ea3055674be_reckful_26660278144_734937575/chunked/
	//are valid results
	baseURLEnd := "903cba256ea3055674be_reckful_26660278144_734937575/chunked/"
	//Same with m3u8 Link
	m3u8LinkEnd := "/903cba256ea3055674be_reckful_26660278144_734937575/chunked/index-dvr.m3u8"
	if err != nil || edgecastBaseURL[len(edgecastBaseURL)-len(baseURLEnd):] != baseURLEnd || m3u8Link[len(m3u8Link)-len(m3u8LinkEnd):] != m3u8LinkEnd {
		t.Errorf("Error in AccessUsherAPI, got baseUrl: %s, m3u8Link: %s", edgecastBaseURL, m3u8Link)
	}
}
