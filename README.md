# SPOTITUBE

[![](https://raw.githubusercontent.com/streambinder/spotitube/master/assets/sample.gif)](#spotitube)

## What is

This project was born as per two needs:

1. I wanted to learn some _GO-lang_ basics.
2. I needed to automate the process of synchronize the songs I wanted to download. This process is composed by several phases:

  - Keep track of music I want to download
  - Find the best song file I can
  - Download it
  - Apply correct metadata

_spotitube_ basically solves these two major problems in a simple, elegant, but especially rapid way.

### How does it work

The solution I wrote to automate the process is covered by two major components:

1. _Spotify_

  This component, once authenticated, is used to keep track of the music to synchronize (both via library or a playlist) and as database for the metadata to apply to every downloaded _mp3_.

2. _YouTube_:

  This one is our free music shop, used to be queried to give us the best video it owns about the songs we're looking for. Once found, that one gets downloaded using a combination of `youtube-dl` and `ffmpeg` commands.

## What does it need

As already mentioned it heavily uses `youtube-dl` to download tracks from _YouTube_ and `ffmpeg` to convert them to _mp3_. You absolutely need them. Thus, it's written in `GO-lang`: assure you actually own it.

Dependency   |       Version        | Dependency type
------------ | :------------------: | :-------------:
`youtube-dl` | _none in particular_ |     Runtime
`ffmpeg`     | _none in particular_ |     Runtime
`golang`     |         1.7+         |   Compilation

## What about its reliability

Several tests got made during the drawing up of the application and now I can say its pretty good at choosing the right song out of a list of keywords (such as the title and the user of any _YouTube_ video).

### Latest statistics

Latest verified statistics describes a sample of 396 songs, cumulative of different musical genres: _rock_, _pop_, _disco_ - _house_, _dubstep_ and _remixes_ -, _chamber music_, _soundtrack_, _folk_, _indie_, _punk_, and many others. Also, they belonged to several decades, with songs from 1975 or up to 2017\. They were produced by many and very different artists, such as _Kodaline_, _Don Diablo_, _OneRepublic_, _The Cinematic Orchestra_, _Sigur Ros_, _Rooney_, _Royal Blood_, _Antonello Venditti_, _Skrillex_, _Savant_, _Knife Party_, _Yann Tiersen_, _Celine Dion_, _The Lumineers_, _alt-J_, _Mumford & Sons_, _Patrick Park_, _Jake Bugg_, _About Wayne_, _Arctic Monkeys_, _The Offspring_, _Maitre Gims_, _Thegiornalisti_, _Glee_ cast, _One Direction_, _Baustelle_, _Kaleo_, _La La Land_ cast, and many, many more.

The result of `spotitube` execution:

Type               | Quantity (of 396)
------------------ | :---------------:
Songs _not found_  |      **13**
Found, but _wrong_ |      **22**
Found, and _right_ |      **361**

In other words, we could say `spotitube` behaved as it was expected to both for _songs not found_ and _found, and right_. In fact, in the first case, the greatest part of the _not found_ songs were actually really not found on _YouTube_.

Type    | Percentage
------- | :--------:
Success |  **95%**
Failure |   **5%**

**PS** The code can surely be taught to behave always better, but there will always be a small percentage of failures, caused by the _YouTube_ users/uploaders, which are unable to specify what a video actually is containing and synthesize it in a title that is not ambiguous (I'm thinking about, for example, the case of a really talented teenager who posts his first cover video, without specifying that it actually is a cover). The more you'll get involved on improve `spotitube`, the more you'll notice how lot of things are ambigous and thinking of a way to workaround this ambiguity would bring the project to be too much selective, losing useful results.

### How to install

#### Download package

Actually, only _RPM_-based and _DEB_-based distributions are supported. If you're on one of those, download the package to [latest release](https://github.com/streambinder/spotitube/releases/latest) page.

#### Build it yourself

The way to build it is pretty straightforward:

```bash
git clone https://github.com/streambinder/spotitube
cd spotitube
make
# to install system-wide
sudo make install
# otherwise you'll find the binary inside ./bin
```

### How to use

```bash
# to download your music library
spotitube -folder ~/Music
# to download a specific $USERNAME accessible playlist
spotitube -folder ~/Music -playlist spotify:user:$USERNAME:playlist:$PLAYLIST_ID
```

#### Additional flags

You may want to use some of the following input flags:

1. `-disable-normalization`: disable songs volume normalization. Although volume normalization is really useful, as lot of songs gets downloaded with several `max_volume` values, resulting into some of them with very low volume level, this option (enabled by default) make the process slow down.
2. `-disable-m3u`: disable automatic creation of `.m3u` file, used to keep track of playlists songs.
3. `-disable-timestamp-flush`: disable automatic songs files timestamps flush to keep library/playlist order.
4. `-disable-update-check`: disable automatic update check at startup.
5. `-interactive`: enable interactive mode. This allows to eventually override `spotitube` decisions about which _YouTube_ result to pick, prompting for user input on every - legal - song it encounters.
6. `-flush-metadata`: enable metadata informations flush also for songs that have been already synchronized.
7. `-replace-local`: replace already downloaded (via `spotitube`) songs, if better ones get encountered.
8. `-clean-junks`: forcely batch remove temporary files that kept existing for any unattended runtime error.
9. `-version`: just print installed version.

#### Developers

For developers, maybe two additional flags could be really useful to simplify the troubleshooting and bugfixing process:

1. `-log` will append every output line of the application to a logfile, located inside the `-folder` the music is getting synchronized in.
2. `-debug` will show additional and detailed messages about the flow that brought the code to choose a song, instead of another, for example. Also, this flag will disable parallelism, in order to have a clearer and more ordered output.
3. `-simulate` will make the process flow till the download step, without proceeding on its way. It's useful to check if searching for results and filtering steps are doing their job.
