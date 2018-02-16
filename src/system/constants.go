package system

import (
	"fmt"
	"time"
)

var (
	DEFAULT_LOG_PATH = fmt.Sprintf("spotitube_%s.log", time.Now().Format("2006-01-02_15.04.05"))
)

const (
	VERSION            = 14
	VERSION_REPOSITORY = "https://github.com/streambinder/spotitube"
	VERSION_ORIGIN     = "https://api.github.com/repos/streambinder/spotitube/releases/latest"
	VERSION_URL        = VERSION_REPOSITORY + "/releases/latest"

	CONCURRENCY_LIMIT = 100

	DEFAULT_EXTENSION    = ".mp3"
	DEFAULT_TCP_CHECK    = "github.com:443"
	DEFAULT_HTTP_TIMEOUT = 3 // second(s)

	SYSTEM_LETTER_BYTES    = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	SYSTEM_LETTER_IDX_BITS = 6
	SYSTEM_LETTER_IDX_MASK = 1<<SYSTEM_LETTER_IDX_BITS - 1
	SYSTEM_LETTER_IDX_MAX  = 63 / SYSTEM_LETTER_IDX_BITS

	LYRICS_API_URL = "https://api.lyrics.ovh/v1/%s/%s"

	YOUTUBE_QUERY_URL          = "https://www.youtube.com/results"
	YOUTUBE_QUERY_PATTERN      = YOUTUBE_QUERY_URL + "?q=%s"
	YOUTUBE_VIDEO_SELECTOR     = ".yt-uix-tile-link"
	YOUTUBE_DESC_SELECTOR      = ".yt-lockup-byline"
	YOUTUBE_DURATION_SELECTOR  = ".accessible-description"
	YOUTUBE_DURATION_TOLERANCE = 20 // second(s)
	YOUTUBE_VIDEO_PREFIX       = "https://www.youtube.com"

	SPOTIFY_CLIENT_ID     = ":SPOTIFY_CLIENT_ID:"
	SPOTIFY_CLIENT_SECRET = ":SPOTIFY_CLIENT_SECRET:"

	SPOTIFY_REDIRECT_URI              = "http://localhost:8080/callback"
	SPOTIFY_FAVICON_URL               = "https://raw.githubusercontent.com/streambinder/spotitube/master/assets/images/spotify.ico"
	SPOTIFY_HTML_AUTOCLOSE_TIMEOUT    = "5"                                    // s
	SPOTIFY_HTML_AUTOCLOSE_TIMEOUT_MS = SPOTIFY_HTML_AUTOCLOSE_TIMEOUT + "000" // ms
	SPOTIFY_HTML_SIG_AUTHOR           = "streambinder"
	SPOTIFY_HTML_SIG_ICON             = "https://www.davidepucci.it/assets/img/profile.png"
	SPOTIFY_HTML_TEMPLATE             = `<!DOCTYPE html>
<html>
<head>
	<meta http-equiv=\"Content-Type\" content=\"text/html; charset=utf-8\">
	<title>SpotiTube</title>
	<link rel="icon" href="` + SPOTIFY_FAVICON_URL + `" type="image/x-icon" />
	<link rel="stylesheet" href="https://maxcdn.bootstrapcdn.com/font-awesome/4.7.0/css/font-awesome.min.css" />
	<link rel="stylesheet" href="https://fonts.googleapis.com/css?family=Roboto+Condensed" />
	<style>
		body               { font: 20px 'Roboto Condensed', sans-serif; text-align: center; margin: 75px 0; padding: 50px; color: #333; border: solid 5px #1ED760; border-left: none; border-right: none; }
		body *             { text-align: center; }
		h1                 { font-size: 40px; text-transform: uppercase; }
		h1>i.fa            { margin: auto 10px; }
		article            { display: block; text-align: left; width: 650px; margin: 0 auto 50px; }
		a                  { color: #dc8100; text-decoration: none; }
		a:hover            { color: #333; text-decoration: none; }
		p.timer            { font-size: 14px; color: #A0A0A0; text-align: center; text-transform: uppercase; }
		div.signature      { border: 1px solid rgba(0, 0, 0, 0.05); border-radius: 5px; text-align: center; }
		div.signature>img  { width: 35px; vertical-align: middle; }
		div.signature>span { font-size: 15px; color: #505050; }
	</style>
	<script type="text/javascript">
		var timeleft = ` + SPOTIFY_HTML_AUTOCLOSE_TIMEOUT + `;
		var downloadTimer = setInterval(function() {
			timeleft--;
			document.getElementById("timer").textContent = timeleft;
			if(timeleft <= 0)
				clearInterval(downloadTimer);
		}, 1000);
		function setAutoClose() { window.setTimeout(autoClose, ` + SPOTIFY_HTML_AUTOCLOSE_TIMEOUT_MS + `); }
		function autoClose() { window.close(); }
	</script>
</head>
<body onLoad="setAutoClose()">
	<article>
		<h1><i class="fa fa-thumbs-up" aria-hidden="true"></i><br>%s</h1>
		<div>
			<h3>%s</h3>
			<br><br><br>
			<p class="timer">Window will attempt to close in <span id="timer">` + SPOTIFY_HTML_AUTOCLOSE_TIMEOUT + `</span> seconds.</p>
			<br>
			<div class="signature">
				<img src="` + SPOTIFY_HTML_SIG_ICON + `"/>
				<span>` + SPOTIFY_HTML_SIG_AUTHOR + `</span>
			</div>
		</div>
	</article>
</body>
</html>`
)
