package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
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

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func getTemplate(srv *drive.Service, parentID string) (string, error) {
	driveID := "0ALBeAJlc9jfJUk9PVA"
	name := "TEMPLATE"
	r, err := srv.
		Files.
		List().
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		DriveId(driveID).
		Corpora("drive").
		Q(fmt.Sprintf(`
			mimeType = 'application/vnd.google-apps.document' and
			name = 'TEMPLATE' and
			'%s' in parents
		`, parentID)).
		Fields("files(id)").
		Do()
	if err != nil {
		return "", err
	}
	if len(r.Files) == 0 {
		return "", fmt.Errorf(`The file %s is not present in the drive`, name)
	}
	if len(r.Files) > 1 {
		return "", fmt.Errorf(`The file %s is present in the drive more than once`, name)
	}
	return r.Files[0].Id, nil
}

func getPIRFolderID(srv *drive.Service) (string, error) {
	name := "Post Incident Review"
	driveID := "0ALBeAJlc9jfJUk9PVA"
	r, err := srv.
		Files.
		List().
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		DriveId(driveID).
		Corpora("drive").
		Q(fmt.Sprintf(`
			mimeType = 'application/vnd.google-apps.folder' and
			name = '%s' and
			'%s' in parents
		`, name, driveID)).
		Fields("files(id)").
		Do()
	if err != nil {
		return "", err
	}
	if len(r.Files) == 0 {
		return "", fmt.Errorf(`The folder %s is not present in the drive`, name)
	}
	if len(r.Files) > 1 {
		return "", fmt.Errorf(`The folder %s is present in the drive more than once`, name)
	}
	return r.Files[0].Id, nil
}

func main() {
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v\n", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, drive.DriveMetadataReadonlyScope, drive.DriveScope, drive.DriveFileScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v\n", err)
	}
	client := getClient(config)

	srv, err := drive.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Drive client: %v\n", err)
	}

	fID, err := getPIRFolderID(srv)
	if err != nil {
		log.Fatalln(err)
	}

	tID, err := getTemplate(srv, fID)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(tID)
}

/*
Notes:

-- This is the address of our shared drive
{"id":"0ALBeAJlc9jfJUk9PVA","name":"Shithappens"}

-- This is the address of the main folder
{"id":"1sSr_A9oe2cIBkzkPiaxszOGaVvIK3Ivd","name":"Post Incident Review","parents":["0ALBeAJlc9jfJUk9PVA"]}

-- In the main folder must be a proto-template. This is the address of the proto-template
{"id":"1Zpve93M7sKOiLKymJBtW0I5hL8rO9NJC-Bw--LfVaEQ","name":"TEMPLATE","parents":["1me_VEAleCPUR69RWu67eaGRFeLmCCHpc"]}

f := drive.File{}
f.DriveId = driveID
f.MimeType = "application/vnd.google-apps.folder"
f.Name = name
f.Parents = []string{driveID}
r, err := srv.Files.Create(&f).Fields("*").Do()
if err != nil {
	log.Fatalf("Unable to create getPIRfolder: %v\n", err)
}
w, _ := r.MarshalJSON()
fmt.Println(string(w))
*/
