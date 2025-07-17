# Email2Folder

A program for automatic processing of emails from a POP3 server: downloads emails, saves them into folders by sender, adds metadata, and processes them using OpenAI.

---

## Features

- Connects to POP3 with login and password authentication
- Downloads and parses emails in .eml format
- Creates folders based on email senders
- Saves emails and attachments to corresponding folders
- Sets custom file attributes (xattr)
- Generates additional information with OpenAI (analysis, tags, summary)
- Runs automatically on a schedule
- Detects binary updates and restarts the service

---

## Installation and Usage

1. Clone the repository and build the binary:

```bash
go build
```

2. Configure the settings in the config file (`conf` package):

- `Addresses` — list of email addresses to process
- `Passwords` — paths to files containing passwords for these addresses
- `Folder` — root directory for saving emails
- `OpenAIToken` — OpenAI API token (optional; disables AI features if missing)
- `StartTimeSecond` — delay before the first run, in seconds

---

## Configuration Parameters

Parameters are defined in the `conf` package:

- **Addresses**: a list of POP3 email addresses to fetch
- **Passwords**: file paths with corresponding passwords
- **Folder**: base directory to store downloaded emails
- **OpenAIToken**: API token for OpenAI features
- **StartTimeSecond**: scheduling delay in seconds

---

## OpenAI Integration

When an OpenAI token is provided, emails will be processed to:

- Determine email type
- Generate a concise summary
- Extract relevant tags
- Identify unsubscribe links if available

---

## File and Folder Structure

- Emails are saved as `.eml` files
- Attachments are stored in the same sender-specific folder
- Filenames use the format `YYYY-MM-DD HH:MM Subject.eml` (safe filename)
- Folders are named after the sender’s email address

---

## Dependencies

- [go-message/mail](https://github.com/emersion/go-message) — email header parsing
- [jhillyerd/enmime](https://github.com/jhillyerd/enmime) — MIME parsing and attachments
- [aurora](https://github.com/logrusorgru/aurora) — colored console output
- Internal modules:
  - `conf` — configuration handling
  - `email` — POP3 client and email utilities
  - `file` — file operations and xattr
  - `openai` — OpenAI API client

---

## Workflow

1. Load configuration
2. Discover email addresses and passwords
3. Connect to the POP3 server
4. Fetch emails and process each:
   - Parse headers and extract body text
   - Find or create the sender folder
   - Save the `.eml` file and any attachments
   - Write custom file attributes (metadata)
   - Call OpenAI API for tags and summary (if enabled)
   - Delete the email from the server
5. Wait until the next scheduled run
6. Restart the service if the binary is updated

---

## Timeouts and Scheduling

- Default connection and processing timeout: **10 seconds**
- Scheduling interval: defined by **StartTimeSecond** (seconds)

---

## Logging

All events and errors are logged to the console with timestamped entries.

---

## License

License: MIT