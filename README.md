<p align="center">
  <img src="docs/logo.png" width="96" height="96" alt="Sortwise logo">
</p>

<h1 align="center">Sortwise</h1>

<p align="center">Your X bookmarks, organized and searchable. Private, on your own computer.</p>

---

Sortwise saves your X (Twitter) bookmarks into a library on your PC, then uses AI to summarize, tag, and file every post, so you can find it again months later.

## Features

- **Saves as you bookmark.** Bookmark a post on X and it is in your library a second later, with its full text, images, video, quoted post, and links.
- **Organizes itself.** Each post gets a short summary, specific tags, and a category, in any field you save from: code, cooking, finance, fitness, design, and more.
- **Finds anything.** Search across post text, summaries, tags, quotes, and link titles. Filter by category and tag.
- **Stays yours.** Your library is one file on your computer. No account, no cloud database, no tracking. Export to JSON or CSV any time.
- **Free to run.** Works with free API keys from Google Gemini and Groq, and switches to the other one when a free limit runs out.

## Install

**You need:** Windows 10 or 11, and Chrome or Microsoft Edge signed in to X.

1. Download the latest release and unzip it.
2. Run `sortwise.exe`. Sortwise opens in your browser and adds an icon to the system tray (right-click it to open or quit). Windows may warn that the app is from an unknown publisher; choose **More info → Run anyway**.
3. Add the extension: open `chrome://extensions` (or `edge://extensions`), turn on **Developer mode**, choose **Load unpacked**, and select the `extension` folder.
4. In Sortwise, go to **Settings → Browser extension** and choose **Create code**. Click the extension's toolbar icon and enter the code.
5. The extension asks what to save:
   - **Only from now on** — leaves out the bookmarks you already have on X.
   - **All my X bookmarks** — brings in every post you've bookmarked, then keeps up.
6. In **Settings → AI organization**, add a free [Gemini](https://aistudio.google.com/apikey) key. A [Groq](https://console.groq.com/keys) key is optional and is used as a backup when Gemini's daily limit runs out.

That's it. From now on, every post you bookmark on X is saved and organized automatically.

## How it works

- **Bookmarking on X** saves the post right away and shows a small "Saved to Sortwise" note. If Sortwise isn't running, the extension keeps the post and saves it next time.
- **Opening X** quietly checks for bookmarks you made elsewhere, such as on your phone, at most once an hour.
- **Sync now** in the extension popup checks on demand. **Everything** re-reads your whole list and continues where it stopped if interrupted.
- **Unbookmarking on X** marks the post *Unbookmarked on X*; it stays in your library. Posts you delete in Sortwise stay deleted unless you bookmark them on X again.
- **AI organizing** works through new posts first. Free keys have daily limits, so a large first import can take a few days. **Pause organizing** in Settings stops it at any time.

## Privacy

Your library, search index, and settings stay on your computer. API keys are kept in Windows Credential Manager and never shown again.

The only thing sent anywhere is the content of each post (its text, up to four images, and link previews), and only to the AI service you chose, to summarize and tag it. Your X login never leaves your browser: the extension reads your bookmarks through X's own page and passes the posts to Sortwise at `127.0.0.1`. There is no telemetry.

## FAQ

**The extension won't pair.**
Make sure Sortwise is running (its tray icon is visible). Codes expire after five minutes and work once, so create a new one.

**Bookmarking a post on X doesn't save it.**
Check that **Save automatically** is on in the popup, then reload your X tabs. Tabs opened before the extension was installed or reloaded aren't connected.

**Sync says X didn't load my bookmarks.**
Make sure you're signed in to X in the same browser, open your bookmarks on X once, and try again.

**Posts are stuck on "Queued".**
Open **Settings → AI organization** and check that a key is added and **Test** succeeds. When a free daily limit is reached, organizing resumes on its own the next day; **Activity** shows what's waiting.

**Can I change what Sortwise saves later?**
Yes. Open the extension popup and choose **Change** next to "Saving …".

**Port 8787 is already in use.**
Start Sortwise with `sortwise.exe --port 8788`, then set **App address** in the extension's **Connection** section to `http://127.0.0.1:8788/api/v1`.

**Where is my library stored, and how do I back it up?**
In `%LOCALAPPDATA%\Sortwise`. **Settings → Your data** creates a backup or exports to JSON or CSV. To restore a backup, quit Sortwise and run `sortwise.exe restore <backup file>`.

**Where's the log?**
`%LOCALAPPDATA%\Sortwise\sortwise.log`.

## Build from source

Requires Go 1.24+ and Node.js 24+.

```powershell
npm install
npm run build
go build -trimpath -ldflags "-s -w -H=windowsgui" -o sortwise.exe ./cmd/sortwise
```

Run the tests with `go test ./...` and `npm run test:ci`. To make a release ZIP, run `scripts/build-release.ps1`.
