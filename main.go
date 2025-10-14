package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type Student struct {
	ID        int       `json:"id"`
	StudentID string    `json:"student_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Submission struct {
	ID          int       `json:"id"`
	StudentID   string    `json:"student_id"`
	ProblemID   int       `json:"problem_id"`
	Code        string    `json:"code"`
	Language    string    `json:"language"`
	Passed      bool      `json:"passed"`
	Score       int       `json:"score"`
	SubmittedAt time.Time `json:"submitted_at"`
}

type TestResult struct {
	Passed   bool        `json:"passed"`
	Expected interface{} `json:"expected"`
	Output   interface{} `json:"output"`
	Error    string      `json:"error,omitempty"`
}

type SubmitResponse struct {
	Results     []TestResult `json:"results"`
	AllPassed   bool         `json:"all_passed"`
	PassedCount int          `json:"passed_count"`
	TotalTests  int          `json:"total_tests"`
	MaxScore    int          `json:"max_score"`
	EarnedScore int          `json:"earned_score"`
}

type ProblemInfo struct {
	ID         int    `json:"id"`
	Title      string `json:"title"`
	Difficulty string `json:"difficulty"`
	MaxScore   int    `json:"max_score"`
}

var db *sql.DB

func main() {
	connStr := "postgres://neondb_owner:npg_1n8oXzslMHyW@ep-ancient-glade-ad22vrvn-pooler.c-2.us-east-1.aws.neon.tech/neondb?sslmode=require&channel_binding=require"

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatal("Cannot connect to database:", err)
	}
	log.Println("Connected to database successfully!")

	createTables()

	http.HandleFunc("/api/register", enableCORS(registerStudent))
	http.HandleFunc("/api/submit", enableCORS(submitCode))
	http.HandleFunc("/api/progress/", enableCORS(getProgress))
	http.HandleFunc("/api/leaderboard", enableCORS(getLeaderboard))
	http.HandleFunc("/api/problems", enableCORS(getProblems))

	fmt.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func createTables() {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS students (
			id SERIAL PRIMARY KEY,
			student_id VARCHAR(50) UNIQUE NOT NULL,
			name VARCHAR(100) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS submissions (
			id SERIAL PRIMARY KEY,
			student_id VARCHAR(50) NOT NULL,
			problem_id INT NOT NULL,
			code TEXT NOT NULL,
			language VARCHAR(20) NOT NULL,
			passed BOOLEAN DEFAULT FALSE,
			score INT DEFAULT 0,
			submitted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (student_id) REFERENCES students(student_id)
		)`,
	}

	for _, query := range queries {
		_, err := db.Exec(query)
		if err != nil {
			log.Printf("Error creating table: %v", err)
		}
	}
}

func enableCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ✅ Allow any origin
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// ✅ Allow standard methods
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

		// ✅ Allow headers
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// ✅ Preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next(w, r)
	}
}

func registerStudent(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var student Student
	if err := json.NewDecoder(r.Body).Decode(&student); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	query := `INSERT INTO students (student_id, name) VALUES ($1, $2) 
	          ON CONFLICT (student_id) DO UPDATE SET name = $2 
	          RETURNING id, student_id, name, created_at`

	err := db.QueryRow(query, student.StudentID, student.Name).Scan(
		&student.ID, &student.StudentID, &student.Name, &student.CreatedAt)

	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Registration failed"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"student": student})
}

func getProblems(w http.ResponseWriter, r *http.Request) {
	problems := []ProblemInfo{
		{ID: 0, Title: "Two Sum", Difficulty: "Easy", MaxScore: 10},
		{ID: 1, Title: "Array Sum", Difficulty: "Easy", MaxScore: 10},
		{ID: 2, Title: "Find Smallest", Difficulty: "Easy", MaxScore: 10},
		{ID: 3, Title: "Rotate Image", Difficulty: "Medium", MaxScore: 20},
		{ID: 4, Title: "Move Zeroes", Difficulty: "Easy", MaxScore: 10},
		{ID: 5, Title: "Single Number", Difficulty: "Medium", MaxScore: 15},
		{ID: 6, Title: "Longest Common Prefix", Difficulty: "Easy", MaxScore: 10},
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"problems": problems,
	})
}

func submitCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		StudentID string `json:"student_id"`
		ProblemID int    `json:"problem_id"`
		Code      string `json:"code"`
		Language  string `json:"language"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	results := runTestCases(req.ProblemID, req.Code, req.Language)

	passedCount := 0
	for _, result := range results {
		if result.Passed {
			passedCount++
		}
	}

	allPassed := passedCount == len(results)
	maxScore := getProblemMaxScore(req.ProblemID)
	earnedScore := (passedCount * maxScore) / len(results)

	query := `INSERT INTO submissions (student_id, problem_id, code, language, passed, score) 
	          VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := db.Exec(query, req.StudentID, req.ProblemID, req.Code, req.Language, allPassed, earnedScore)
	if err != nil {
		log.Printf("Error saving submission: %v", err)
	}

	response := SubmitResponse{
		Results:     results,
		AllPassed:   allPassed,
		PassedCount: passedCount,
		TotalTests:  len(results),
		MaxScore:    maxScore,
		EarnedScore: earnedScore,
	}

	respondJSON(w, http.StatusOK, response)
}

func getProblemMaxScore(problemID int) int {
	scores := map[int]int{
		0: 10, 1: 10, 2: 10, 3: 20, 4: 10, 5: 15, 6: 10,
	}
	return scores[problemID]
}

func runTestCases(problemID int, code string, language string) []TestResult {
	testCases := getTestCases(problemID)
	results := make([]TestResult, len(testCases))

	for i, tc := range testCases {
		output, err := executeCode(problemID, code, language, tc.input)
		if err != nil {
			results[i] = TestResult{
				Passed:   false,
				Expected: tc.expected,
				Output:   nil,
				Error:    err.Error(),
			}
			continue
		}

		passed := compareOutput(output, tc.expected)
		results[i] = TestResult{
			Passed:   passed,
			Expected: tc.expected,
			Output:   output,
		}
	}

	return results
}

type testCase struct {
	input    string
	expected interface{}
}

func getTestCases(problemID int) []testCase {
	switch problemID {
	case 0: // Two Sum
		return []testCase{
			{`[2,7,11,15],9`, []int{0, 1}},
			{`[3,2,4],6`, []int{1, 2}},
			{`[3,3],6`, []int{0, 1}},
			{`[1,5,3,7,9],12`, []int{2, 4}},
			{`[10,20,30,40],50`, []int{1, 2}},
		}
	case 1: // Array Sum
		return []testCase{
			{`[1,2,3,4,5]`, 15},
			{`[10,20,30]`, 60},
			{`[-1,-2,-3]`, -6},
			{`[0,0,0,0]`, 0},
			{`[100]`, 100},
		}
	case 2: // Find Smallest
		return []testCase{
			{`[3,1,4,1,5,9,2,6]`, 1},
			{`[10,5,20,8]`, 5},
			{`[-5,-10,-3]`, -10},
			{`[100]`, 100},
			{`[7,7,7,7,7]`, 7},
		}
	case 3: // Rotate Image
		return []testCase{
			{`[[1,2,3],[4,5,6],[7,8,9]]`, [][]int{{7, 4, 1}, {8, 5, 2}, {9, 6, 3}}},
			{`[[5,1,9,11],[2,4,8,10],[13,3,6,7],[15,14,12,16]]`, [][]int{{15, 13, 2, 5}, {14, 3, 4, 1}, {12, 6, 8, 9}, {16, 7, 10, 11}}},
			{`[[1]]`, [][]int{{1}}},
			{`[[1,2],[3,4]]`, [][]int{{3, 1}, {4, 2}}},
			{`[[2,29,20,26],[16,28,15,17],[9,4,5,19]]`, [][]int{{9, 16, 2}, {4, 28, 29}, {5, 15, 20}, {19, 17, 26}}},
		}
	case 4: // Move Zeroes
		return []testCase{
			{`[0,1,0,3,12]`, []int{1, 3, 12, 0, 0}},
			{`[0]`, []int{0}},
			{`[1,2,3]`, []int{1, 2, 3}},
			{`[0,0,1]`, []int{1, 0, 0}},
			{`[2,1,0,0,4,0,5]`, []int{2, 1, 4, 5, 0, 0, 0}},
		}
	case 5: // Single Number
		return []testCase{
			{`[2,2,1]`, 1},
			{`[4,1,2,1,2]`, 4},
			{`[1]`, 1},
			{`[7,3,5,3,7]`, 5},
			{`[10,20,10,30,20]`, 30},
		}
	case 6: // Longest Common Prefix
		return []testCase{
			{`["flower","flow","flight"]`, "fl"},
			{`["dog","racecar","car"]`, ""},
			{`["interspecies","interstellar","interstate"]`, "inters"},
			{`["prefix","preface","preview"]`, "pre"},
			{`["alone"]`, "alone"},
		}
	}
	return []testCase{}
}

func executeCode(problemID int, code string, language string, input string) (interface{}, error) {
	switch language {
	case "python":
		return executePython(problemID, code, input)
	case "javascript":
		return executeJavaScript(problemID, code, input)
	case "golang":
		return executeGo(problemID, code, input)
	}
	return nil, fmt.Errorf("language not supported")
}

func executePython(problemID int, code string, input string) (interface{}, error) {
	funcName := getPythonFuncName(problemID)
	fullCode := code + "\nimport json\nprint(json.dumps(" + funcName + "(" + input + ")))"

	cmd := exec.Command("python3", "-c", fullCode)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("execution error: %s", string(output))
	}

	return parseOutput(string(output), problemID)
}

func executeJavaScript(problemID int, code string, input string) (interface{}, error) {
	funcName := getJSFuncName(problemID)
	fullCode := code + "\nconsole.log(JSON.stringify(" + funcName + "(" + input + ")));"

	cmd := exec.Command("node", "-e", fullCode)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("execution error: %s", string(output))
	}

	return parseOutput(string(output), problemID)
}

func executeGo(problemID int, code string, input string) (interface{}, error) {
	return nil, fmt.Errorf("Go execution not fully implemented in this demo")
}

func getPythonFuncName(problemID int) string {
	names := []string{"two_sum", "array_sum", "find_smallest", "rotate_image", "move_zeroes", "single_number", "longest_common_prefix"}
	return names[problemID]
}

func getJSFuncName(problemID int) string {
	names := []string{"twoSum", "arraySum", "findSmallest", "rotateImage", "moveZeroes", "singleNumber", "longestCommonPrefix"}
	return names[problemID]
}

func parseOutput(output string, problemID int) (interface{}, error) {
	output = strings.TrimSpace(output)

	// Problems that return arrays or 2D arrays
	if problemID == 0 || problemID == 4 {
		var result []int
		err := json.Unmarshal([]byte(output), &result)
		if err != nil {
			return nil, fmt.Errorf("invalid output format")
		}
		return result, nil
	}

	// Problem 3 returns 2D array
	if problemID == 3 {
		var result [][]int
		err := json.Unmarshal([]byte(output), &result)
		if err != nil {
			return nil, fmt.Errorf("invalid output format")
		}
		return result, nil
	}

	// Problem 6 returns string
	if problemID == 6 {
		var result string
		err := json.Unmarshal([]byte(output), &result)
		if err != nil {
			return nil, fmt.Errorf("invalid output format")
		}
		return result, nil
	}

	// Problems 1, 2, 5 return integers
	var result int
	_, err := fmt.Sscanf(output, "%d", &result)
	if err != nil {
		return nil, fmt.Errorf("invalid output format")
	}
	return result, nil
}

func compareOutput(output interface{}, expected interface{}) bool {
	outputJSON, _ := json.Marshal(output)
	expectedJSON, _ := json.Marshal(expected)
	return string(outputJSON) == string(expectedJSON)
}

func getProgress(w http.ResponseWriter, r *http.Request) {
	studentID := strings.TrimPrefix(r.URL.Path, "/api/progress/")

	query := `SELECT DISTINCT problem_id, MAX(score) as max_score FROM submissions 
	          WHERE student_id = $1 
	          GROUP BY problem_id`

	rows, err := db.Query(query, studentID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch progress"})
		return
	}
	defer rows.Close()

	type ProblemProgress struct {
		ProblemID int `json:"problem_id"`
		Score     int `json:"score"`
	}

	var progress []ProblemProgress
	totalScore := 0
	for rows.Next() {
		var p ProblemProgress
		if err := rows.Scan(&p.ProblemID, &p.Score); err == nil {
			progress = append(progress, p)
			totalScore += p.Score
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"progress":    progress,
		"total_score": totalScore,
	})
}

func getLeaderboard(w http.ResponseWriter, r *http.Request) {
	query := `SELECT s.student_id, s.name, 
	          COALESCE(SUM(sub.max_score), 0) as total_score
	          FROM students s
	          LEFT JOIN (
	              SELECT student_id, problem_id, MAX(score) as max_score
	              FROM submissions
	              GROUP BY student_id, problem_id
	          ) sub ON s.student_id = sub.student_id
	          GROUP BY s.student_id, s.name
	          ORDER BY total_score DESC, s.name ASC
	          LIMIT 50`

	rows, err := db.Query(query)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch leaderboard"})
		return
	}
	defer rows.Close()

	type LeaderboardEntry struct {
		StudentID string `json:"student_id"`
		Name      string `json:"name"`
		Score     int    `json:"score"`
	}

	var leaderboard []LeaderboardEntry
	for rows.Next() {
		var entry LeaderboardEntry
		if err := rows.Scan(&entry.StudentID, &entry.Name, &entry.Score); err == nil {
			leaderboard = append(leaderboard, entry)
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"leaderboard": leaderboard,
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
