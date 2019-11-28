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

func getDocumentID(srv *drive.Service, parentID, docName string) (string, error) {
	driveID := "0ALBeAJlc9jfJUk9PVA"
	r, err := srv.
		Files.
		List().
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		DriveId(driveID).
		Corpora("drive").
		Q(fmt.Sprintf(`
			mimeType = 'application/vnd.google-apps.document' and
			'%s' in parents and
			name = '%s'
		`, parentID, docName)).
		Fields("files(id)").
		Do()
	if err != nil {
		return "", err
	}
	if len(r.Files) == 0 {
		return "", fmt.Errorf(`The file %s is not present in the drive`, docName)
	}
	if len(r.Files) > 1 {
		return "", fmt.Errorf(`The file %s is present in the drive more than once`, docName)
	}
	return r.Files[0].Id, nil
}

func getFolderList(srv *drive.Service, parentID, folderName string) (*drive.FileList, error) {
	driveID := "0ALBeAJlc9jfJUk9PVA"
	fileList, err := srv.
		Files.
		List().
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		DriveId(driveID).
		Corpora("drive").
		Q(fmt.Sprintf(`
			mimeType = 'application/vnd.google-apps.folder' and
			'%s' in parents and
			name = '%s'
		`, parentID, folderName)).
		Fields("files(id)").
		Do()
	if err != nil {
		return nil, err
	}
	return fileList, nil
}

func getFolderID(srv *drive.Service, parentID, folderName string) (string, error) {
	fileList, err := getFolderList(srv, parentID, folderName)
	if err != nil {
		return "", err
	}
	if len(fileList.Files) == 0 {
		return "", fmt.Errorf(`The folder %s is not present in the drive`, folderName)
	}
	if len(fileList.Files) > 1 {
		return "", fmt.Errorf(`The folder %s is present in the drive more than once`, folderName)
	}
	return fileList.Files[0].Id, nil
}

func getOrCreateTeamFolderID(srv *drive.Service, PIRID, teamName string) (string, error) {
	fileList, err := getFolderList(srv, PIRID, teamName)
	if err != nil {
		return "", err
	}
	if len(fileList.Files) == 0 {
		f := drive.File{}
		f.MimeType = "application/vnd.google-apps.folder"
		f.Name = teamName
		f.Parents = []string{PIRID}
		createdFile, err := srv.
			Files.
			Create(&f).
			SupportsAllDrives(true).
			Fields("id").
			Do()
		if err != nil {
			return "", err
		}
		return createdFile.Id, nil
	}
	if len(fileList.Files) > 1 {
		return "", fmt.Errorf(`The folder %s is present in the drive more than once`, teamName)
	}
	return fileList.Files[0].Id, nil
}

func getOrCreateDocumentInTeamID(srv *drive.Service, teamID, baseTemplateID, docName string) (string, error) {
	templateInTeamID, err := getDocumentID(srv, teamID, docName)
	if err != nil {
		f := drive.File{}
		f.MimeType = "application/vnd.google-apps.document"
		f.Name = docName
		f.Parents = []string{teamID}
		r, err := srv.
			Files.
			Copy(baseTemplateID, &f).
			SupportsAllDrives(true).
			Fields("id").
			Do()
		if err != nil {
			return "", err
		}
		return r.Id, nil
	}
	return templateInTeamID, nil
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

	fPirID, err := getFolderID(srv, "0ALBeAJlc9jfJUk9PVA", "Post Incident Review")
	if err != nil {
		log.Fatalln(err)
	}

	baseTemplateID, err := getDocumentID(srv, fPirID, "TEMPLATE")
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(baseTemplateID, "template base")

	teamName := "Team test squad 5"
	teamID, err := getOrCreateTeamFolderID(srv, fPirID, teamName)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(teamID, "team")

	templateInTeamID, err := getOrCreateDocumentInTeamID(srv, teamID, baseTemplateID, "TEMPLATE")
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(templateInTeamID, "template in team")

	incidentName := "Incident at Shithappens - Testing Postmortem"
	postMortemInTeamID, err := getOrCreateDocumentInTeamID(srv, teamID, templateInTeamID, incidentName)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(postMortemInTeamID, "post-mortem review")
}

/*
Notes:

-- This is the address of our shared drive
{"id":"0ALBeAJlc9jfJUk9PVA","name":"Shithappens"}

*/
