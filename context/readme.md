1. Problem Definition

Manual job application is inefficient.

Typical workflow:

Search jobs on Naukri

Open job

Read job description

Check skill match

Click apply

Fill form

Upload resume

Submit

This takes 1–2 minutes per job.

Applying to 50 jobs = 1–1.5 hours.

The system should automate:

search jobs
↓
filter jobs
↓
evaluate job match
↓
auto fill application
↓
submit

The goal is targeted automation, not blind spam.

2. System Goals

Primary goals:

Automatically discover jobs from Naukri

Evaluate job description relevance using AI

Automatically apply to matching jobs

Automatically fill application forms

Run locally with zero cost

Easy configuration for users

3. System Constraints

These constraints matter.

Cost

Must run locally.

No paid APIs.

Use:

Ollama (local LLM)

Playwright

SQLite

Anti Bot Protection

Naukri has basic protection.

Solution:

use real browser (Playwright chromium)

slow interaction

human-like delays

Resume Upload

Forms vary.

Need dynamic handling for:

text inputs
dropdowns
radio buttons
file upload
4. Target Users

Users:

developers applying to jobs

job seekers

students

User requirement:

Provide information via config file.

Example:

profile.yaml

Contains:

name
email
phone
skills
experience
location
resume path
5. Core Features

Version 1 must include only these:

Job discovery

Scrape jobs from search results.

AI job matching

Evaluate:

job description
vs
user profile

Return match score.

Application automation

Open job page.

Click apply.

Fill form.

Upload resume.

Submit.

Job deduplication

Avoid applying to same job twice.

Local execution

User runs:

docker run job-auto-apply

System starts applying.

6. High Level Architecture (HLD)

System architecture:

                 ┌───────────────┐
                 │ User Profile  │
                 │  profile.yaml │
                 └───────┬───────┘
                         │
                         ▼
                 ┌───────────────┐
                 │ Job Scraper   │
                 │ (Playwright)  │
                 └───────┬───────┘
                         │
                         ▼
                 ┌───────────────┐
                 │ Job Parser    │
                 │ Extract data  │
                 └───────┬───────┘
                         │
                         ▼
                 ┌───────────────┐
                 │ AI Job Filter │
                 │ (Ollama LLM)  │
                 └───────┬───────┘
                         │
                         ▼
                 ┌───────────────┐
                 │ Apply Worker  │
                 │ (Playwright)  │
                 └───────┬───────┘
                         │
                         ▼
                 ┌───────────────┐
                 │ Form Handler  │
                 │ Auto Fill     │
                 └───────┬───────┘
                         │
                         ▼
                 ┌───────────────┐
                 │ SQLite DB     │
                 │ Job history   │
                 └───────────────┘
7. Service Architecture

You have two choices.

Option A (Recommended)

Single service.

Go service

Handles everything.

Advantages:

simple

easier debugging

fewer moving parts

Option B

Microservices.

scraper service
AI service
apply service

Overkill for this project.

Stick with single service.

8. Technology Stack

Everything free.

Backend language:

Go

Why Go?

concurrency

fast scraping

good CLI tools

single binary deployment

Automation:

Playwright

AI:

Ollama
Llama3

Database:

SQLite

Configuration:

YAML

Container:

Docker
9. Data Flow

Full workflow:

Load profile.yaml
↓
Start scraper
↓
Fetch job list pages
↓
Extract job links
↓
Open job page
↓
Parse job description
↓
Send to AI matcher
↓
Calculate match score
↓
If score >= threshold
↓
Send to apply worker
↓
Fill form
↓
Upload resume
↓
Submit
↓
Save result in DB
10. AI Matching System

AI must answer:

Does this job match this candidate?

Input:

candidate profile
job description

Output:

score 0–100

Prompt design:

You are a technical recruiter.

Evaluate if the following job matches the candidate.

Candidate Skills:
Node.js
Express
MySQL
Kafka
Redis

Candidate Experience:
2 years backend development

Job Description:
{JOB_DESCRIPTION}

Return only JSON:

{
 "match_score": number,
 "reason": "short explanation"
}
11. Form Automation Logic

Forms differ.

You need a generic form handler.

Steps:

detect inputs

match input label

fill value

Example mapping:

Name → profile.name
Email → profile.email
Phone → profile.phone
Experience → profile.experience

Algorithm:

for each form field
  read label text
  map to profile field
  fill value
12. Database Design

SQLite schema:

jobs
----
id
title
company
url
description
match_score
status
applied_at

Status:

pending
applied
skipped
failed
13. Folder Structure

Recommended:

job-auto-apply/

cmd/
   main.go

internal/

   config/
      loader.go

   scraper/
      naukri_scraper.go

   ai/
      matcher.go

   apply/
      apply_worker.go

   form/
      form_filler.go

   db/
      sqlite.go

configs/
   profile.yaml

resume/
   resume.pdf

docker/
   Dockerfile
14. Execution Flow

User runs:

docker run job-auto-apply

Steps:

load profile
↓
start scraper
↓
scrape jobs
↓
AI evaluate
↓
apply worker
↓
form filler
↓
submit
15. Security Considerations

Avoid storing credentials in code.

Use:

.env

Example:

NAUKRI_EMAIL=
NAUKRI_PASSWORD=
16. Failure Handling

Common failures:

form changed

captcha

login expired

System should:

retry
log error
skip job
17. Future Extensions

Later you can add:

LinkedIn
Indeed
Instahyre

Also:

resume auto tailoring
job recommendation engine
analytics dashboard

But not now.