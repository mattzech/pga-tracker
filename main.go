package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strconv"
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
}

type LeaderboardResponse struct {
	LeaderboardRows []LeaderboardRow `json:"leaderboardRows"`
}

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

	names, err := loadTeam("teams/matt.json")
	if err != nil {
		log.Fatal(err)
	}

	team, err := getTeamScores("leaderboard.json", names)
	if err != nil {
		log.Fatal(err)
	}

	for _, p := range team {
		fmt.Printf("%s: R1=%d R2=%d R3=%d R4=%d Total=%s\n", p.FullName, p.R1, p.R2, p.R3, p.R4, p.Total)
	}
}

func getTeamScores(filePath string, teamNames []string) ([]Player, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var leaderboard LeaderboardResponse
	if err := json.NewDecoder(file).Decode(&leaderboard); err != nil {
		return nil, err
	}

	var team []Player
	for _, row := range leaderboard.LeaderboardRows {
		fullName := row.FirstName + " " + row.LastName
		if !slices.Contains(teamNames, fullName) {
			continue
		}

		player := Player{
			FullName: fullName,
			Total:    row.Total,
		}

		for i, round := range row.Rounds {
			switch i {
			case 0:
				player.R1 = strokesInt(round.Strokes)
			case 1:
				player.R2 = strokesInt(round.Strokes)
			case 2:
				player.R3 = strokesInt(round.Strokes)
			case 3:
				player.R4 = strokesInt(round.Strokes)
			}
		}

		team = append(team, player)
	}
	return team, nil
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
