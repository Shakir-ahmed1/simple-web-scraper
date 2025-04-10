package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/joho/godotenv"
)

var baseURL string
var foundUrlFileName string
var scrapedUrlFileName string
var downloadedFilesFolderName string

func ensureFoldersAndFiles() {
	os.MkdirAll("site_pages", os.ModePerm)
	for _, f := range []string{"found_urls.txt", "scraped_urls.txt"} {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			os.WriteFile(f, []byte(""), 0644)
		}
	}
}

func readLines(filepath string) ([]string, error) {
	var lines []string
	file, err := os.Open(filepath)
	if err != nil {
		return lines, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func appendLineIfNotExists(filepath, line string) error {
	lines, err := readLines(filepath)
	if err != nil {
		return err
	}
	for _, l := range lines {
		if l == line {
			return nil
		}
	}
	f, err := os.OpenFile(filepath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

func getStartIndex(found, scraped []string) int {
	if len(scraped) == 0 {
		return 0
	}
	last := scraped[len(scraped)-1]
	for i, url := range found {
		if url == last && i+1 < len(found) {
			return i + 1
		}
	}
	return len(found)
}

func sanitizeFilename(url string) string {
	return strings.ReplaceAll(strings.TrimPrefix(url, baseURL), "/", "_")
}

func extractLinksFromHTML(html string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}
	var links []string
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			href = strings.TrimSpace(href)
			if strings.HasPrefix(href, "/") {
				href = baseURL + href[1:]
			}
			if strings.HasPrefix(href, baseURL) {
				links = append(links, href)
			}
		}
	})
	return links, nil
}

func scrapeAndSave(url string, index int) ([]string, error) {
	fmt.Println("Scraping:", url)

	// Define client with custom redirect policy (follow redirects)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// You can log redirects here if needed
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil // follow redirect
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 OPR/106.0.0.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	fileName := fmt.Sprintf("%d.html", index)
	fmt.Println("file name", fileName)
	if fileName == "" {
		fileName = "index"
	}
	filePath := filepath.Join("site_pages", fileName+".html")
	err = ioutil.WriteFile(filePath, bodyBytes, 0644)
	if err != nil {
		return nil, err
	}

	liveLinks, err := extractLinksFromHTML(string(bodyBytes))
	if err != nil {
		return nil, err
	}

	savedData, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	localLinks, err := extractLinksFromHTML(string(savedData))
	if err != nil {
		return nil, err
	}

	allLinks := append(liveLinks, localLinks...)
	return allLinks, nil
}

func storeURLs(urls []string) {
	for _, url := range urls {
		_ = appendLineIfNotExists("found_urls.txt", url)
	}
}

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Get the BASE_URL environment variable
	baseURL = os.Getenv("BASE_URL")
	foundUrlFileName = os.Getenv("FOUND_URLS_FILENAME")
	scrapedUrlFileName = os.Getenv("SCRAPED_URLS_FILENAME")
	downloadedFilesFolderName = os.Getenv("DOWNLOADED_FILES_FOLDERNAME")
	if baseURL == "" {
		log.Fatal("BASE_URL is not set in the environment variables")
	}

	// Now baseURL can be used throughout your program
	fmt.Println("Base URL:", baseURL)

	// Ensure folders and files, then proceed with scraping logic
	ensureFoldersAndFiles()
	baseURLString := []string{baseURL}
	storeURLs(baseURLString)

	for {
		// Read the lines from the files
		foundURLs, _ := readLines("found_urls.txt")
		scrapedURLs, _ := readLines("scraped_urls.txt")

		// If both files have the same number of lines, exit the loop
		if len(foundURLs) == len(scrapedURLs) {
			fmt.Println("Scraping completed successfully ✅")
			break
		}

		// Determine the starting index for scraping
		startIndex := getStartIndex(foundURLs, scrapedURLs)

		// If all URLs have been scraped, exit the loop
		if startIndex >= len(foundURLs) {
			fmt.Println("Scraping completed successfully ✅")
			break
		}

		// Scrape the URLs starting from the current index
		for i := startIndex; i < len(foundURLs); i++ {
			url := foundURLs[i]

			newLinks, err := scrapeAndSave(url, i)
			if err != nil {
				fmt.Println("Failed to scrape", url, ":", err)
				continue
			}

			// Store new links found during scraping
			storeURLs(newLinks)

			// Add the current URL to scraped_urls.txt if not already present
			_ = appendLineIfNotExists("scraped_urls.txt", url)
		}

		// Pause before the next iteration to allow updates to the files
		time.Sleep(1 * time.Second) // Adjust the duration as necessary
	}
}
