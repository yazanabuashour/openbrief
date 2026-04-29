package runner

import (
	"encoding/xml"
	"fmt"
	"strings"
)

type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	GUID    string `xml:"guid"`
	PubDate string `xml:"pubDate"`
	Source  struct {
		Text string `xml:",chardata"`
	} `xml:"source"`
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string     `xml:"title"`
	ID      string     `xml:"id"`
	Updated string     `xml:"updated"`
	Links   []atomLink `xml:"link"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

func parseFeed(body []byte) ([]fetchedItem, error) {
	var rss rssFeed
	if err := xml.Unmarshal(body, &rss); err == nil && rss.XMLName.Local == "rss" {
		items := make([]fetchedItem, 0, len(rss.Channel.Items))
		for _, item := range rss.Channel.Items {
			title := cleanText(item.Title)
			link := strings.TrimSpace(item.Link)
			identity := strings.TrimSpace(item.GUID)
			if identity == "" {
				identity = link
			}
			if identity == "" {
				identity = title
			}
			if title != "" {
				items = append(items, fetchedItem{Title: title, URL: link, PublishedAt: strings.TrimSpace(item.PubDate), Identity: identity, FeedIdentity: identity, RSSSource: item.Source.Text})
			}
		}
		return items, nil
	}
	var atom atomFeed
	if err := xml.Unmarshal(body, &atom); err != nil {
		return nil, fmt.Errorf("parse feed XML: %w", err)
	}
	if atom.XMLName.Local != "feed" {
		return nil, fmt.Errorf("parse feed XML: unsupported feed root %q", atom.XMLName.Local)
	}
	items := make([]fetchedItem, 0, len(atom.Entries))
	for _, entry := range atom.Entries {
		title := cleanText(entry.Title)
		link := atomEntryLink(entry)
		identity := strings.TrimSpace(entry.ID)
		if identity == "" {
			identity = link
		}
		if identity == "" {
			identity = title
		}
		if title != "" {
			items = append(items, fetchedItem{Title: title, URL: link, PublishedAt: strings.TrimSpace(entry.Updated), Identity: identity, FeedIdentity: identity})
		}
	}
	return items, nil
}

func atomEntryLink(entry atomEntry) string {
	if len(entry.Links) == 0 {
		return ""
	}
	for _, link := range entry.Links {
		if link.Rel == "" || link.Rel == "alternate" {
			return strings.TrimSpace(link.Href)
		}
	}
	return strings.TrimSpace(entry.Links[0].Href)
}

func cleanText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
