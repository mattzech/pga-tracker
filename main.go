package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type PageData struct {
	Teams       []Team
	LastUpdated string
}

type Team struct {
	TeamName   string   `json:"teamName"`
	Players    []string `json:"players"`
	PlayerScores []Player `json:"-"`
	History    []string `json:"history"`
}

type Player struct {
	FullName string `json:"name"`
	R1       int    `json:"r1"`
	R2       int    `json:"r2"`
	R3       int    `json:"r3"`
	R4       int    `json:"r4"`
	Total    int    `json:"total"`
	Excluded bool
}

type Round struct {
	Strokes string `json:"scoreToPar"`
}

type LeaderboardRow struct {
	FirstName string  `json:"firstName"`
	LastName  string  `json:"lastName"`
	Total     string  `json:"total"`
	Rounds    []Round `json:"rounds"`
	Position  string  `json:"position"`
}

type Leaderboard struct {
	CutLines []struct {
		CutScore string `json:"cutScore"`
	} `json:"cutLines"`
	LeaderboardRows []LeaderboardRow `json:"leaderboardRows"`
}

var (
	members = []string{"Matt", "JR", "Pat", "Alex", "Chuck"}
)

func main() {
	refresh := flag.Bool("refresh", false, "Fetch latest leaderboard from API")
	flag.Parse()

	if *refresh {
		err := fetchLeaderboard()
		if err != nil {
			log.Fatalf("Failed to refresh leaderboard: %v", err)
		}
		log.Println("✅ Fetched latest leaderboard")
	}
   teams := make([]Team, len(members))
   for i, member := range members {
	   teamData, err := loadTeam(fmt.Sprintf("teams/%s.json", member))
	   if err != nil {
		   log.Fatal(err)
	   }

	   playerScores, err := getTeamScores("leaderboard.json", teamData.Players)
	   if err != nil {
		   log.Fatal(err)
	   }

	   teams[i] = teamData
	   teams[i].PlayerScores = playerScores
   }

   err := renderScoreboard(teams)
   if err != nil {
	   log.Fatalf("render failed: %v", err)
   }
}

func getTeamScores(filePath string, teamNames []string) ([]Player, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var leaderboard Leaderboard
	if err := json.NewDecoder(file).Decode(&leaderboard); err != nil {
		return nil, err
	}

	cutVal := 0
	if len(leaderboard.CutLines) > 0 {
		cutVal = parseCutScore(leaderboard.CutLines[0].CutScore) + 3
	}

	var team []Player
	for _, name := range teamNames {
		firstName, lastName := splitName(name)
		var found *LeaderboardRow
		for _, row := range leaderboard.LeaderboardRows {
			if row.FirstName == firstName && row.LastName == lastName {
				found = &row
				break
			}
		}
		if found == nil {
			log.Printf("Player not found in leaderboard: %s", name)
			continue
		}

		player := Player{FullName: name}
		isCut := strings.ToUpper(found.Position) == "CUT"

		for i := 0; i < 4; i++ {
			if i < len(found.Rounds) && !isCut {
				strokes := strokesInt(found.Rounds[i].Strokes)
				switch i {
				case 0:
					player.R1 = strokes
				case 1:
					player.R2 = strokes
				case 2:
					player.R3 = strokes
				case 3:
					player.R4 = strokes
				}
			} else if isCut && i >= 2 {
				switch i {
				case 2:
					player.R3 = cutVal
				case 3:
					player.R4 = cutVal
				}
			}
		}
  if len(found.Rounds) == 0 {
	player.R1 = strokesInt(found.Total)
  }
		player.Total = player.R1 + player.R2 + player.R3 + player.R4
		team = append(team, player)
	}

	sort.Slice(team, func(i, j int) bool {
		return team[i].Total < team[j].Total
	})

	for i := 4; i < len(team); i++ {
		team[i].Excluded = true
	}

	r1Total, r2Total, r3Total, r4Total, grandTotal := 0, 0, 0, 0, 0
	for _, p := range team[:4] {
		r1Total += p.R1
		r2Total += p.R2
		r3Total += p.R3
		r4Total += p.R4
		grandTotal += p.Total
	}

	total := Player{
		FullName: "Total",
		R1:       r1Total,
		R2:       r2Total,
		R3:       r3Total,
		R4:       r4Total,
		Total:    grandTotal,
	}
	team = append(team, total)

	return team, nil
}



func splitName(name string) (string, string) {
	switch name {
	case "Min Woo Lee":
		return "Min Woo", "Lee"
	}
	split := strings.SplitN(name, " ", 2)
	if len(split) != 2 {
		log.Printf("Skipping invalid name: %s", name)
	}
	firstName, lastName := split[0], split[1]
	return firstName, lastName
}

func playerTotal(p Player) int {
	return p.R1 + p.R2 + p.R3 + p.R4
}

func parseCutScore(cut string) int {
	val := strings.TrimPrefix(cut, "+")
	val = strings.TrimPrefix(val, "-")
	var score int
	fmt.Sscanf(val, "%d", &score)
	return score
}

func loadTeam(filePath string) (Team, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return Team{}, err
	}
	defer file.Close()

	var team Team
	if err := json.NewDecoder(file).Decode(&team); err != nil {
		return Team{}, err
	}
	return team, nil
}

func fetchLeaderboard() error {
	apiKey := os.Getenv("RAPID_GOLF_API_KEY")

	url := "https://live-golf-data.p.rapidapi.com/leaderboard?orgId=1&tournId=026&year=2025"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Add("x-rapidapi-key", apiKey)
	req.Header.Add("x-rapidapi-host", "live-golf-data.p.rapidapi.com")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d %s", res.StatusCode, res.Status)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("Failed to read response body: %v", err)
	}

	// Optional: Pretty-print JSON to a file
	var prettyJSON map[string]interface{}
	if err := json.Unmarshal(body, &prettyJSON); err != nil {
		return fmt.Errorf("Failed to parse JSON: %v", err)
	}

	file, err := os.Create("leaderboard.json")
	if err != nil {
		return fmt.Errorf("Failed to create file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") // Pretty-print with indent
	if err := encoder.Encode(prettyJSON); err != nil {
		return fmt.Errorf("Failed to write JSON to file: %v", err)
	}

	fmt.Println("✅ Saved leaderboard data to leaderboard.json")
	return nil
}

func strokesInt(s string) int {
	strokes, _ := strconv.Atoi(s)
	return strokes
}

func renderScoreboard(teams []Team) error {
	tmpl := template.Must(template.New("scoreboard").Funcs(template.FuncMap{
		"isTotal": func(name string) bool {
			return name == "Total"
		},
	}).ParseFiles("templates/scoreboard.html"))
	

	out, err := os.Create("docs/index.html")
	if err != nil {
		return err
	}
	defer out.Close()

	data := PageData{
		Teams:       teams,
		LastUpdated: time.Now().Format("Jan 2, 2006 3:04PM MST"),
	}
	

	return tmpl.Execute(out, data)
}
