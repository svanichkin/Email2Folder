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
- Classifies each email with DeepSeek V4.1 Flash using its complete original `.eml` source
- Generates Russian Markdown and metadata with DeepSeek V4.1 Flash via OpenRouter (summary, tags, unsubscribe link)
- Holds an IMAP `IDLE` connection and processes new mail immediately
- Reconnects automatically after an IMAP error

---

## Installation and Usage

1. Clone the repository and build the binary:

```bash
go build
```

2. Configure the settings in the config file (`conf` package):

- `Addresses` — list of email addresses to process
- `Passwords` — paths to files containing passwords for these addresses
- `Keys` — directory containing secret key files
- `Folder` — root directory for saving emails
- `RemoteAIToken` — RemoteAI token filename inside `Keys` (for example, `remoteai`); when `Keys` is omitted, the value itself is used for backwards compatibility
- `IMAPServer` — IMAP hostname; if omitted, the domain part of the username is used
- `IMAPProcessingMailbox` — shared IMAP mailbox used to claim messages; when omitted, the service creates `Email2Folder Processing` inside the account's personal IMAP namespace (for example `INBOX.Email2Folder Processing`)

---

## Configuration Parameters

Parameters are defined in the `conf` package:

- **Addresses**: a list of IMAP email addresses to fetch
- **Passwords**: file paths with corresponding passwords
- **Keys**: directory with secret key files
- **Folder**: base directory to store downloaded emails
- **RemoteAIToken**: API token filename in `Keys`, or the token itself if `Keys` is unset
- **IMAPServer**: IMAP hostname, useful when it differs from the email domain
- **IMAPProcessingMailbox**: shared mailbox; every worker must use the same value. Leave it empty to use the account's personal IMAP namespace automatically.

---

## RemoteAI Integration

When an OpenRouter token is provided, emails will be processed to:

- Determine the email type with `deepseek/deepseek-v4.1-flash`. DeepSeek receives the complete original raw RFC 5322/MIME `.eml`, including headers and MIME structure.
- Generate a clean Russian Markdown representation from the raw EML
- Generate a concise summary, relevant tags, and an unsubscribe link where available

DeepSeek performs classification, Russian Markdown generation, and metadata extraction in one bounded request. The email is saved and deleted from IMAP only after this complete JSON response has been parsed and validated. DeepSeek reasoning is disabled so the completion budget is reserved for the requested JSON.

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
   - Send the complete raw `.eml` to DeepSeek once for classification, Russian Markdown, and metadata (if enabled)
   - Delete the claimed email from the server only after all files are saved
5. Enter IMAP `IDLE` and wait for a new-message notification
6. Process newly arrived messages immediately; reconnect after an IMAP error

---

## Timeouts and IDLE

- Connection setup timeout: **10 seconds**
- Each DeepSeek request has a **45-second** deadline
- The service stays in IMAP `IDLE`; it refreshes the IDLE command every 29 minutes and reconnects after IMAP errors

---

## Logging

All events and errors are logged to the console with timestamped entries.

---

## License

License: MIT
