package spotitube

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey"
	"github.com/PuerkitoBio/goquery"
)

type YouTube struct {
	Interactive bool
}

type YouTubeTrack struct {
	Track    Track
	ID       string
	URL      string
	Title    string
	User     string
	Duration int
}

func NewYouTubeClient() *YouTube {
	return &YouTube{
		Interactive: false,
	}
}

func (youtube *YouTube) SetInteractive(set_interactive bool) {
	youtube.Interactive = set_interactive
}

func (youtube *YouTube) FindTrack(track Track) (YouTubeTrack, error) {
	var doc *goquery.Document
	logger.Log("Searching youtube results to \"" + fmt.Sprintf(YOUTUBE_QUERY_PATTERN, strings.Replace(track.SearchPattern, " ", "+", -1)) + "\".")
	request, _ := http.NewRequest("GET", fmt.Sprintf(YOUTUBE_QUERY_PATTERN, strings.Replace(track.SearchPattern, " ", "+", -1)), nil)
	request.Header.Add("Accept-Language", "en")
	response, err := http.DefaultClient.Do(request)
	if err == nil {
		doc, _ = goquery.NewDocumentFromResponse(response)
	} else {
		doc, err = goquery.NewDocument(fmt.Sprintf(YOUTUBE_QUERY_PATTERN, strings.Replace(track.SearchPattern, " ", "+", -1)))
	}
	if err != nil {
		logger.Warn("Cannot retrieve doc from \"" + fmt.Sprintf(YOUTUBE_QUERY_PATTERN, strings.Replace(track.SearchPattern, " ", "+", -1)) + "\": " + err.Error())
		return YouTubeTrack{}, err
	}
	// html, _ := doc.Html()
	// logger.Debug(html)
	selection := doc.Find(YOUTUBE_VIDEO_SELECTOR)
	selection_desc := doc.Find(YOUTUBE_DESC_SELECTOR)
	selection_duration := doc.Find(YOUTUBE_DURATION_SELECTOR)
	for lap := range [2]int{} {
		if youtube.Interactive && lap > 0 {
			continue
		}
		for selection_item := range selection.Nodes {
			item := selection.Eq(selection_item)
			item_href, item_href_ok := item.Attr("href")
			item_title, item_title_ok := item.Attr("title")
			item_user, item_user_ok := "UNKNOWN", false
			item_length, item_length_ok := 0, false
			if selection_item < len(selection_desc.Nodes) {
				item_desc := selection_desc.Eq(selection_item)
				item_user = strings.TrimSpace(item_desc.Find("a").Text())
				item_user_ok = true
			}
			if selection_item < len(selection_duration.Nodes) {
				var item_length_m, item_length_s int
				item_duration := selection_duration.Eq(selection_item)
				item_length_str := strings.TrimSpace(item_duration.Text())
				if strings.Contains(item_length_str, ": ") {
					item_length_str = strings.Split(item_length_str, ": ")[1]
					item_length_m, err = strconv.Atoi(strings.Split(item_length_str, ":")[0])
					if err == nil {
						item_length_s, err = strconv.Atoi(strings.Split(item_length_str, ":")[1][:2])
						if err == nil {
							item_length = item_length_m*60 + item_length_s
							item_length_ok = true
						}
					}
				}
			}
			if !(item_href_ok && item_title_ok && item_user_ok && item_length_ok) {
				logger.Debug("Non-standard YouTube video entry structure: " +
					"url is " + strconv.FormatBool(item_href_ok) + ", " +
					"title is " + strconv.FormatBool(item_title_ok) + ", " +
					"user is " + strconv.FormatBool(item_user_ok) + ", " +
					"duration is " + strconv.FormatBool(item_length_ok) + ". Continuing scraping...")
				continue
			} else if !strings.Contains(strings.ToLower(item_href), "youtu.be") &&
				!strings.Contains(strings.ToLower(item_href), "watch?v=") {
				logger.Debug("Advertising URL found. Continuing scraping...")
				continue
			}

			youtube_track := YouTubeTrack{
				Track:    track,
				ID:       IdFromUrl(YOUTUBE_VIDEO_PREFIX + item_href),
				URL:      YOUTUBE_VIDEO_PREFIX + item_href,
				Title:    item_title,
				User:     item_user,
				Duration: item_length,
			}

			logger.Debug("ID: " + youtube_track.ID +
				" | URL: " + youtube_track.URL +
				" | Title: " + youtube_track.Title +
				" | User: " + youtube_track.User +
				" | Duration: " + fmt.Sprintf("%d", youtube_track.Duration))

			ans := false
			ans_automated := (lap == 0 && youtube_track.Match(track, true)) ||
				(lap == 1 && youtube_track.Match(track, false))
			if youtube.Interactive {
				var ans_automated_msg string
				if ans_automated {
					ans_automated_msg = "I would do it"
				} else {
					ans_automated_msg = "I wouldn't do it"
				}
				prompt := &survey.Confirm{
					Message: "Do you want to download " + youtube_track.User +
						"'s video \"" + youtube_track.Title + "\" at \"" + youtube_track.URL +
						"\" (" + ans_automated_msg + ")?",
				}
				survey.AskOne(prompt, &ans, nil)
				if !ans {
					continue
				}
			}

			if ans || ans_automated {
				track.URL = youtube_track.URL
				return youtube_track, nil
			}
		}
	}

	logger.Warn("YouTube video URL not found. Dropping song download.")
	return YouTubeTrack{}, errors.New("YouTube video URL not found")
}

func (youtube_track YouTubeTrack) Match(track Track, strict bool) bool {
	item_title := strings.ToLower(youtube_track.Title)

	if int(math.Abs(float64(track.Duration-youtube_track.Duration))) > YOUTUBE_DURATION_TOLERANCE {
		logger.Debug(fmt.Sprintf("The duration difference is excessive: | %d - %d | = %d (max tolerated: %d)",
			track.Duration, youtube_track.Duration, int(math.Abs(float64(track.Duration-youtube_track.Duration))), YOUTUBE_DURATION_TOLERANCE))
		return false
	}

	if strings.Contains(youtube_track.URL, "&list=") || strings.Contains(youtube_track.URL, "/user/") {
		logger.Debug("Track is actually pointing to playlist or user.")
		return false
	} else if track.Seems(youtube_track.Title) {
		logger.Debug("Song seems the one we're looking for. Checking youtube specific stuff.")
		if strict &&
			(strings.Contains(item_title, "official video") ||
				(strings.Contains(youtube_track.User, "VEVO") &&
					!(strings.Contains(item_title, "audio") || strings.Contains(item_title, "lyric")))) {
			logger.Debug("First page readup, temporarily ignoring \"" + youtube_track.Title + "\" by \"" + youtube_track.User + "\".")
			return false
		}
		logger.Log("Video \"" + youtube_track.Title + "\" matches with track \"" + track.Artist + " - " + track.Title + "\".")
		return true
	}

	return false
}

func (track YouTubeTrack) Download() error {
	logger.Log("Going to download \"" + track.URL + "\" to \"" + track.Track.FilenameTemporary() + "\".")
	command_cmd := "youtube-dl"
	command_args := []string{"--output", track.Track.FilenameTemp + ".%(ext)s", "--format", "bestaudio", "--extract-audio", "--audio-format", track.Track.FilenameExt[1:], "--audio-quality", "0", track.URL}
	_, err := exec.Command(command_cmd, command_args...).Output()
	if err != nil {
		logger.Warn("Something went wrong while executing \"" + command_cmd + " " + strings.Join(command_args, " ") + "\": " + err.Error())
		return err
	}
	logger.Log("Song downloaded to: \"" + track.Track.FilenameTemporary() + "\".")
	return nil
}

func IdFromUrl(url string) string {
	var id_part string
	if strings.Contains(strings.ToLower(url), "youtu.be/") {
		id_part = strings.Split(url, "youtu.be/")[1]
	} else {
		id_part = strings.Split(url, "watch?v=")[1]
	}
	if strings.Contains(id_part, "?") {
		id_part = strings.Split(id_part, "?")[0]
	}
	if strings.Contains(id_part, "&list") {
		id_part = strings.Split(id_part, "&list")[0]
	}
	return id_part
}
