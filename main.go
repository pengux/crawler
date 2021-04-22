package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/andybalholm/cascadia"
	sitemap "github.com/oxffaa/gopher-parse-sitemap"
	"github.com/urfave/cli/v2"
	"golang.org/x/net/html"
)

var (
	client = http.Client{}
)

func main() {
	app := &cli.App{
		Name:   "crawler",
		Usage:  "Crawl URLs so it can be pre-cached",
		Action: run,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "mode",
				Aliases: []string{"m"},
				Usage:   "The mode for crawling, either 'sitemap' or 'links', defaults to 'sitemap'",
				Value:   "sitemap",
			},
			&cli.StringSliceFlag{
				Name:    "site",
				Aliases: []string{"s"},
				Usage:   "The site to crawl",
			},
			&cli.StringFlag{
				Name:    "css3selector",
				Aliases: []string{"c"},
				Usage:   "The CSS3 selector to use to find links to crawl on the website, default to 'a' for all '<a>' tags",
				Value:   "a",
			},
			&cli.IntFlag{
				Name:    "concurrency",
				Aliases: []string{"n"},
				Usage:   "Number of concurrent crawling processes",
				Value:   10,
			},
			&cli.IntFlag{
				Name:    "timeout",
				Aliases: []string{"t"},
				Usage:   "Request timeout",
				Value:   10,
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func run(c *cli.Context) error {
	sites := c.StringSlice("site")
	client = http.Client{
		Timeout: time.Duration(c.Int("timeout")) * time.Second,
	}
	concurrency := c.Int("concurrency")

	for _, site := range sites {
		if c.String("mode") == "links" {
			selector, err := cascadia.Compile(c.String("css3selector"))
			if err != nil {
				return err
			}

			err = crawlLinks(site, concurrency, selector)
			if err != nil {
				return err
			}
		} else {
			err := crawlSitemap(site+"/sitemap.xml", concurrency)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func crawlSitemap(url string, concurrency int) error {
	log.Printf("crawling sitemap index '%s'", url)
	var sem = make(chan int, concurrency)

	return sitemap.ParseFromSite(url, func(e sitemap.Entry) error {
		sem <- 1
		go func(url string) {
			defer func() {
				<-sem
			}()

			err := do(url)
			if err != nil {
				log.Printf("[ERROR] could not crawl URL '%s': %v", url, err)
			}
		}(e.GetLocation())
		return nil
	})
}

func crawlLinks(startURL string, concurrency int, selector cascadia.Selector) error {
	req, err := http.NewRequest(http.MethodGet, startURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	// Get the links in main menu
	htmlDoc, err := html.Parse(resp.Body)
	if err != nil {
		return err
	}
	err = resp.Body.Close()
	if err != nil {
		return err
	}

	links := selector.MatchAll(htmlDoc)
	var sem = make(chan int, concurrency)

	for _, link := range links {
		for _, attr := range link.Attr {
			if attr.Key != "href" {
				continue
			}

			sem <- 1
			go func(url string) {
				defer func() {
					<-sem
				}()

				if !strings.HasPrefix(url, "http") {
					if !strings.HasPrefix(url, "/") {
						url = "/" + url
					}
					url = startURL + url
				}
				if !strings.HasPrefix(url, startURL) {
					return
				}

				err := do(url)
				if err != nil {
					log.Fatal(err)
				}
			}(attr.Val)
		}
	}

	return nil
}

func do(url string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	start := time.Now()
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	log.Printf("response time: %d ms for requesting %s\n", time.Since(start).Milliseconds(), url)

	return nil
}
