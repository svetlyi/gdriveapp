package auth

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"io/ioutil"
	"os"
	"path/filepath"
)

// GetTokenSource retrieves a token, saves the token, then returns the generated client.
func GetTokenSource(configDirPath string) (oauth2.TokenSource, error) {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := filepath.Join(configDirPath, "token.json")
	cfg, err := readCredsConfig(configDirPath)
	if nil != err {
		return nil, errors.Wrap(err, "could not read config with credentials")
	}
	tok, err := tokenFromFile(tokFile)
	if err != nil { // if could not read token from file, create it
		tok, err = getTokenFromWeb(cfg)
		if nil != err {
			return nil, errors.Wrap(err, "could not get token from web")
		}
		if err = saveToken(tokFile, tok); nil != err {
			return nil, err
		}
	}

	return cfg.TokenSource(context.Background(), tok), nil
}

func readCredsConfig(configDirPath string) (*oauth2.Config, error) {
	credsFilePath := filepath.Join(configDirPath, "credentials.json")
	var (
		b   []byte
		err error
	)
	b, err = ioutil.ReadFile(credsFilePath)
	for err != nil {
		fmt.Printf("Make sure the key file '%s' exists and is readable. Press Enter to continue", credsFilePath)
		bufio.NewReader(os.Stdin).ReadLine()
		b, err = ioutil.ReadFile(credsFilePath)
	}

	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse client config file")
	}

	return config, nil
}

// getTokenFromWeb requests a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) (*oauth2.Token, error) {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, errors.Wrap(err, "unable to read authorization code")
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		return nil, errors.Wrap(err, "unable to retrieve token from web")
	}
	return tok, nil
}

// tokenFromFile retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// saveToken saves a token to a file path.
func saveToken(path string, token *oauth2.Token) error {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return errors.Wrapf(err, "unable to save oauth token to %s", path)
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(token)
}
