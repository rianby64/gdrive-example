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
	"google.golang.org/api/option"
)

var driveID = ""

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

func getDriveList(srv *drive.Service) (*drive.DriveList, error) {
	driveList, err := srv.
		Drives.
		List().
		Fields("*").
		Do()
	if err != nil {
		return nil, err
	}
	return driveList, nil
}

func getFileList(srv *drive.Service, mimeType, parentID, docName string) (*drive.FileList, error) {
	fileList, err := srv.
		Files.
		List().
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		DriveId(driveID).
		Corpora("drive").
		Q(fmt.Sprintf(`
		    trashed = false and
			mimeType = '%s' and
			'%s' in parents and
			name = '%s'
		`, mimeType, parentID, docName)).
		Fields("files(id, webViewLink)").
		Do()
	if err != nil {
		return nil, err
	}
	return fileList, nil
}

func getDocumentID(srv *drive.Service, parentID, docName string) (string, error) {
	fileList, err := getFileList(srv, "application/vnd.google-apps.document", parentID, docName)
	if err != nil {
		return "", err
	}
	if len(fileList.Files) == 0 {
		return "", fmt.Errorf(`The file %s is not present in the drive`, docName)
	}
	if len(fileList.Files) > 1 {
		return "", fmt.Errorf(`The file %s is present in the drive more than once`, docName)
	}
	return fileList.Files[0].Id, nil
}

func getDocumentLinkByID(srv *drive.Service, docID string) (string, error) {
	file, err := srv.
		Files.
		Get(docID).
		SupportsAllDrives(true).
		Fields("webViewLink").
		Do()
	if err != nil {
		return "", err
	}
	return file.WebViewLink, nil
}

func getFolderList(srv *drive.Service, parentID, folderName string) (*drive.FileList, error) {
	fileList, err := getFileList(srv, "application/vnd.google-apps.folder", parentID, folderName)
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
			Fields("id, webViewLink").
			Do()
		if err != nil {
			return "", err
		}
		return r.Id, nil
	}
	return templateInTeamID, nil
}

func authByAUTH0() (*drive.Service, error) {
	b, err := ioutil.ReadFile("oauth.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v\n", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b,
		drive.DriveAppdataScope,
		drive.DriveScope,
		drive.DriveFileScope,
	)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	srv, err := drive.New(client)
	if err != nil {
		return nil, fmt.Errorf("Unable to retrieve Drive client: %v", err)
	}

	return srv, nil
}

func authByToken() (*drive.Service, error) {
	ctx := context.Background()
	token, err := tokenFromFile("token.json")
	if err != nil {
		return nil, fmt.Errorf("Unable to retrieve Token file: %v", err)
	}
	tokenSource := oauth2.StaticTokenSource(token)

	srv, err := drive.NewService(ctx,
		option.WithTokenSource(tokenSource),
		option.WithScopes(
			drive.DriveAppdataScope,
			drive.DriveScope,
			drive.DriveFileScope,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("Unable to retrieve Drive client: %v", err)
	}

	return srv, nil
}

func authByCredentials1() (*drive.Service, error) {
	ctx := context.Background()
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v\n", err)
	}
	creds, err := google.JWTConfigFromJSON(b,
		drive.DriveAppdataScope,
		drive.DriveScope,
		drive.DriveFileScope,
	)
	if err != nil {
		return nil, fmt.Errorf("Unable to retreive Credentials: %v", err)
	}

	client := creds.Client(ctx)

	srv, err := drive.NewService(ctx,
		option.WithHTTPClient(client),
	)
	if err != nil {
		return nil, fmt.Errorf("Unable to retrieve Drive client: %v", err)
	}
	return srv, nil
}

func authByCredentials2() (*drive.Service, error) {
	ctx := context.Background()
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v\n", err)
	}
	creds, err := google.CredentialsFromJSON(ctx, b,
		drive.DriveAppdataScope,
		drive.DriveScope,
		drive.DriveFileScope,
	)
	if err != nil {
		return nil, fmt.Errorf("Unable to retreive Credentials: %v", err)
	}

	srv, err := drive.NewService(ctx,
		option.WithCredentials(creds),
	)
	if err != nil {
		return nil, fmt.Errorf("Unable to retrieve Drive client: %v", err)
	}
	return srv, nil
}

func authByCredentials3() (*drive.Service, error) {
	ctx := context.Background()
	srv, err := drive.NewService(ctx,
		option.WithCredentialsFile("credentials.json"),
		option.WithScopes(
			drive.DriveAppdataScope,
			drive.DriveScope,
			drive.DriveFileScope,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("Unable to retrieve Drive client: %v", err)
	}
	return srv, nil
}

func main() {
	srv, err := authByCredentials1()
	if err != nil {
		log.Fatalln(err)
	}

	/*
		about, err := srv.About.Get().Fields("*").Do()
		if err != nil {
			log.Fatalln(err)
		}
		aboutJSON, _ := about.MarshalJSON()
		fmt.Println(string(aboutJSON))

		driveList, err := getDriveList(srv)
		if err != nil {
			log.Fatalln(err)
		}
		drivesJSON, _ := driveList.MarshalJSON()
		fmt.Println(string(drivesJSON))
	*/
	files, err := srv.
		Files.
		List().
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		Fields("*").
		Do()

	if err != nil {
		log.Fatalln(err)
	}
	filesJSON, _ := files.MarshalJSON()
	fmt.Println(string(filesJSON))

	/*
		fPirID, err := getFolderID(srv, "", "Post Incident Review")
		if err != nil {
			log.Fatalln(err)
		}

		baseTemplateID, err := getDocumentID(srv, fPirID, "TEMPLATE")
		if err != nil {
			log.Fatalln(err)
		}
		fmt.Println(baseTemplateID, "template base")

		teamName := "Team test squad 8"
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

		incidentName := "Incident at Shithappens - Testing Postmortem 3"
		postMortemInTeamID, err := getOrCreateDocumentInTeamID(srv, teamID, templateInTeamID, incidentName)
		if err != nil {
			log.Fatalln(err)
		}
		link, err := getDocumentLinkByID(srv, postMortemInTeamID)
		if err != nil {
			log.Fatalln(err)
		}
		fmt.Println(link, "post-mortem link")
	*/
}
