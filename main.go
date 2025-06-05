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
)

type Player struct {
	FullName string `json:"name"`
	R1       int    `json:"r1"`
	R2       int    `json:"r2"`
	R3       int    `json:"r3"`
	R4       int    `json:"r4"`
	Total    string `json:"total"`
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
	members = []string{"matt", "jr", "pat", "alex", "chuck"}
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
			Total:    found.Total,
		}

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
				// Assign cut penalty strokes to R3 and R4
				switch i {
				case 2:
					player.R3 = cutVal
				case 3:
					player.R4 = cutVal
				}
			}
		}

		team = append(team, player)
	}
	sort.Slice(team, func(i, j int) bool {
		return playerTotal(team[i]) < playerTotal(team[j])
	})

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
	tmpl, err := template.ParseFiles("templates/scoreboard.html")
	if err != nil {
		return err
	}

	out, err := os.Create("docs/index.html")
	if err != nil {
		return err
	}
	defer out.Close()

	return tmpl.Execute(out, teams)
}
