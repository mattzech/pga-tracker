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
    Teams       map[string][]Player
    LastUpdated string
}


type Player struct {
	FullName string `json:"name"`
	R1       int    `json:"r1"`
	R2       int    `json:"r2"`
	R3       int    `json:"r3"`
	R4       int    `json:"r4"`
	Total    int    `json:"total"`
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

	teams := make(map[string][]Player, len(members))
	for _, member := range members {
		playerNames, err := loadTeam(fmt.Sprintf("teams/%s.json", member))
		if err != nil {
			log.Fatal(err)
		}

		team, err := getTeamScores("leaderboard.json", playerNames)
		if err != nil {
			log.Fatal(err)
		}

		teams[member] = team
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
		cutVal = parseCutScore(leaderboard.CutLines[0].CutScore) + 3 // decide a value to assign to cut players. here we are adding 3 to the cutline as their Round score
	}

	var team []Player
	r1Total := 0
	r2Total := 0
	r3Total := 0
	r4Total := 0
	grandTotal := 0
	for _, name := range teamNames {
		split := strings.SplitN(name, " ", 2)
		if len(split) != 2 {
			log.Printf("Skipping invalid name: %s", name)
			continue
		}
		firstName, lastName := split[0], split[1]

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
		player := Player{
			FullName: name,
		}

		isCut := strings.ToUpper(found.Position) == "CUT"

		for i := 0; i < 4; i++ {
			if i < len(found.Rounds) && !isCut {
				strokes := strokesInt(found.Rounds[i].Strokes)
				switch i {
				case 0:
					player.R1 = strokes
					r1Total += strokes
				case 1:
					player.R2 = strokes
					r2Total += strokes
				case 2:
					player.R3 = strokes
					r3Total += strokes
				case 3:
					player.R4 = strokes
					r4Total += strokes
				}
			} else if isCut && i >= 2 {
				// Assign cut penalty strokes to R3 and R4
				switch i {
				case 2:
					player.R3 = cutVal
					r3Total += cutVal
				case 3:
					player.R4 = cutVal
					r4Total += cutVal
				}
			}
		}
		currentPlayerTotal := player.R1 + player.R2 + player.R3 + player.R4
		player.Total = currentPlayerTotal

		team = append(team, player)
	}
	sort.Slice(team, func(i, j int) bool {
		return playerTotal(team[i]) < playerTotal(team[j])
	})
	grandTotal = r1Total + r2Total + r3Total + r4Total
	total := Player{
		FullName: "Total",
		R1:    r1Total,
		R2:    r2Total,
		R3:    r3Total,
		R4:    r4Total,
		Total: grandTotal,
	}
	team = append(team, total)

	return team, nil
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

func loadTeam(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var names []string
	if err := json.NewDecoder(file).Decode(&names); err != nil {
		return nil, err
	}
	return names, nil
}

func fetchLeaderboard() error {
	apiKey := os.Getenv("RAPID_GOLF_API_KEY")

	url := "https://live-golf-data.p.rapidapi.com/leaderboard?orgId=1&tournId=023&year=2025"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Add("x-rapidapi-key", apiKey)
	req.Header.Add("x-rapidapi-host", "live-golf-data.p.rapidapi.com")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Failed to make request: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected status code: %d %s", res.StatusCode, res.Status)
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

func renderScoreboard(teams map[string][]Player) error {
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
