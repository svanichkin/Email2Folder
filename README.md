# Email2Folder

A program for automatic processing of emails from an IMAP server: claims emails safely across multiple machines, saves them into folders by sender, adds metadata, and processes them using RemoteAI.

---

## Features

- Connects to IMAPS (TLS on port 993) with login and password authentication
- Atomically moves each email from `INBOX` to a shared processing mailbox before handling it, so only one machine can claim it
- Downloads and parses emails in .eml format
- Creates folders based on email senders
- Saves emails and attachments to corresponding folders
- Sets custom file attributes (xattr)
- Generates additional information with RemoteAI (analysis, tags, summary)
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
- `RemoteAIToken` — RemoteAI API token (optional; disables AI features if missing)
- `IMAPServer` — IMAP hostname; if omitted, the domain part of the username is used
- `IMAPProcessingMailbox` — shared IMAP mailbox used to claim messages; defaults to `Email2Folder Processing`
- `StartTimeSecond` — delay before the first run, in seconds

---

## Configuration Parameters

Parameters are defined in the `conf` package:

- **Addresses**: a list of IMAP email addresses to fetch
- **Passwords**: file paths with corresponding passwords
- **Folder**: base directory to store downloaded emails
- **RemoteAIToken**: API token for RemoteAI features
- **IMAPServer**: IMAP hostname, useful when it differs from the email domain
- **IMAPProcessingMailbox**: shared mailbox; every worker must use the same value
- **StartTimeSecond**: scheduling delay in seconds

---

## RemoteAI Integration

When an RemoteAI token is provided, emails will be processed to:

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
  - `email` — IMAP client and email utilities
  - `file` — file operations and xattr
  - `remoteai` — RemoteAI API client

---

## Workflow

1. Load configuration
2. Discover email addresses and passwords
3. Connect to the IMAP server
4. Fetch emails and process each:
   - Atomically move it from `INBOX` to the processing mailbox to claim it
   - Parse headers and extract body text
   - Find or create the sender folder
   - Save the `.eml` file and any attachments
   - Write custom file attributes (metadata)
   - Call RemoteAI API for tags and summary (if enabled)
   - Delete the claimed email from the server only after all files are saved
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
