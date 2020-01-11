package main

import (
	"encoding/csv"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

type Area struct {
	Id       string
	Name     string
	Division string
}

type Result struct {
	Division   string
	District   string
	Number     string
	Candidates string
	Ballots    string
	Percentage string
}

type Worker struct {
	sync.WaitGroup
	jobCh    chan *Area
	resultCh chan *Result
}

func NewWorker(n int) *Worker {
	worker := &Worker{
		jobCh:    make(chan *Area),
		resultCh: make(chan *Result),
	}
	for i := 0; i < n; i++ {
		go worker.worker()
	}
	return worker
}

func (w *Worker) Add(area *Area) {
	w.WaitGroup.Add(1)
	w.jobCh <- area
}

func (w *Worker) Wait() {
	w.WaitGroup.Wait()
	close(w.resultCh)
}

func (w *Worker) Result() <-chan *Result {
	return w.resultCh
}

func (w *Worker) worker() {
	for j := range w.jobCh {
		w.doJob(j)
	}
}

func (w *Worker) doJob(area *Area) {
	defer w.WaitGroup.Done()

	doc, err := getDocument(fmt.Sprintf("https://www.cec.gov.tw/pc/zh_TW/P1/n%s.html", area.Id))
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	doc.Find("#divContent .trT").Each(func(i int, row *goquery.Selection) {
		result := &Result{
			Division: area.Division,
			District: area.Name,
		}

		cells := row.Find("td")
		result.Number = cells.Eq(1).Text()
		html, _ := cells.Eq(2).Html()
		result.Candidates = strings.ReplaceAll(html, "<br/>", "/")
		result.Ballots = strings.ReplaceAll(cells.Eq(4).Text(), ",", "")
		result.Percentage = cells.Eq(5).Text()

		w.resultCh <- result
	})
}

func main() {
	table, err := getFolderStructure()
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	w := NewWorker(30)
	ch := w.Result()
	go addJobs(w, table)

	records := [][]string{
		{"縣市", "鄉鎮市區", "號次", "總統/副總統", "得票數", "得票率%"},
	}

	for res := range ch {
		records = append(records, []string{
			res.Division, res.District, res.Number, res.Candidates, res.Ballots, res.Percentage,
		})
	}
	outputCSV(records)
}

func outputCSV(records [][]string) {
	w := csv.NewWriter(os.Stdout)
	for _, record := range records {
		if err := w.Write(record); err != nil {
			log.Fatalln("error writing record to csv:", err)
		}
	}
	w.Flush()

	if err := w.Error(); err != nil {
		log.Fatalf("error: %v\n", err)
	}
}

func addJobs(w *Worker, table map[int]map[int]*Area) {
	for _, row := range table {
		for j, area := range row {
			if j != 0 {
				w.Add(area)
			}
		}
	}
	w.Wait()
}

func getFolderStructure() (map[int]map[int]*Area, error) {
	body, err := getResponseBody("https://www.cec.gov.tw/pc/zh_TW/js/treeP1.js")
	if err != nil {
		return nil, err
	}
	result := make(map[int]map[int]*Area)

	secAreaID := regexp.MustCompile(`secAreaID\[(\d+)\]\[(\d+)\]='(\d+)';`)
	all := secAreaID.FindAllSubmatch(body, -1)
	for _, group := range all {
		i, _ := strconv.Atoi(string(group[1]))
		j, _ := strconv.Atoi(string(group[2]))
		if j == 0 {
			result[i] = make(map[int]*Area)
		}
		result[i][j] = &Area{Id: string(group[3])}
	}

	secAreaName := regexp.MustCompile(`secAreaName\[(\d+)\]\[(\d+)\]='(.+)';`)
	for _, group := range secAreaName.FindAllSubmatch(body, -1) {
		i, _ := strconv.Atoi(string(group[1]))
		j, _ := strconv.Atoi(string(group[2]))
		if j != 0 {
			result[i][j].Division = result[i][0].Name
		}
		result[i][j].Name = string(group[3])
	}
	return result, nil
}

func getResponseBody(url string) ([]byte, error) {
	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return ioutil.ReadAll(resp.Body)
}

func getDocument(url string) (*goquery.Document, error) {
	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return goquery.NewDocumentFromReader(resp.Body)
}
