WaterlooStar BackEnd API (Full)

This document is the contract between backend and frontend.
It explains what to call, what to send, and what to expect back.

1. Base

Base URL
http://localhost:8080

Headers
Use JSON for request bodies:
Content-Type: application/json

Time Format
All times use RFC3339, example: 2024-01-01T00:00:00Z

2. Health

Purpose
Quick check to confirm the server is up.

Endpoint
GET /health

Response 200
ok

3. Auth

3.1 Register

Purpose
Create a new user account.

Endpoint
POST /auth/register

Request body
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "pass123"
}

Field notes
- username: required, unique
- email: required, unique, lowercase is enforced
- password: required, stored as bcrypt hash

Responses
200:
{"status":"ok"}
409:
{"status":"error","error":"duplicate username"}
{"status":"error","error":"duplicate email"}
400:
{"status":"error","error":"..."}

3.2 Login

Purpose
Check credentials and return user info for the session.

Endpoint
POST /auth/login

Request body
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "pass123"
}

Field notes
- username or email: one is required
- password: required

Responses
200:
{
  "status":"ok",
  "user":{
    "id":"<USER_ID>",
    "username":"alice",
    "email":"alice@example.com",
    "verified":false
  }
}
401:
unauthorized

3.3 Send Verification Code

Purpose
Generate a verification code for an existing user.

Endpoint
POST /auth/send-code

Request body
{
  "email":"alice@example.com"
}

Field notes
- email: required

Responses
200:
{"status":"ok"}
401:
unauthorized

Notes
- Codes are stored in memory and expire after 10 minutes.

3.4 Verify Code

Purpose
Validate the code and mark the user as verified.

Endpoint
POST /auth/verify-code

Request body
{
  "email":"alice@example.com",
  "code":"123456"
}

Field notes
- email: required
- code: required

Responses
200:
{"status":"ok"}
401:
unauthorized

4. Posts

4.1 List Posts (simple)

Purpose
Simple list with limit/offset.

Endpoint
GET /posts

Query params
- limit: default 20
- offset: default 0

Response 200
[
  {
    "id":"...",
    "title":"...",
    "body":"...",
    "creator_id":"...",
    "images":[],
    "created_at":"...",
    "updated_at":"...",
    "views":0,
    "likes":0,
    "stars":0,
    "comment_number":0,
    "comments":[]
  }
]

4.2 Create Post

Purpose
Create a new post.

Endpoint
POST /posts

Request body
{
  "title":"Hello",
  "body":"First post",
  "creator_id":"<USER_ID>",
  "images":[]
}

Field notes
- title: required
- body: required
- creator_id: required (user id)
- images: optional list of URLs

Responses
201:
{
  "id":"...",
  "title":"Hello",
  "body":"First post",
  "creator_id":"<USER_ID>",
  "images":[],
  "created_at":"...",
  "updated_at":"...",
  "views":0,
  "likes":0,
  "stars":0,
  "comment_number":0,
  "comments":[]
}
400:
title/body/creator_id is required

4.3 List Posts (page + filters)

Purpose
List posts by page, with filters and sorting.

Endpoint
POST /posts/list

Request body
{
  "page": 1,
  "pageSize": 10,
  "sort": { "created_at": "descend" },
  "filters": {
    "time_from": "2024-01-01T00:00:00Z",
    "time_to": "2024-12-31T23:59:59Z"
  }
}

Field notes
- page: required, 1-based
- pageSize: required
- sort: optional map (only one field is used)
  allowed fields: created_at, views, likes, stars, comment_number
  direction: ascend/descend (defaults to descend)
- filters: optional
  time_from/time_to must be RFC3339

Responses
200:
{
  "meta": {
    "page": 1,
    "pageSize": 10,
    "totalPages": 1,
    "sort": { "created_at": "descend" },
    "filters": {
      "time_from": "2024-01-01T00:00:00Z",
      "time_to": "2024-12-31T23:59:59Z"
    }
  },
  "data": [
    {
      "id":"...",
      "title":"...",
      "excerpt":"...",
      "author": { "id":"...", "name":"alice" },
      "image": [],
      "stats": {
        "views": 0,
        "likes": 0,
        "stars": 0,
        "replies": 0
      },
      "createdAt":"2024-01-01T00:00:00Z",
      "lastUpdatedAt":"2024-01-01T00:00:00Z",
      "flag": { "liked": false, "stared": false }
    }
  ]
}
400:
invalid time_from, expected RFC3339
invalid time_to, expected RFC3339

5. Frontend Examples (fetch)

Register
fetch("http://localhost:8080/auth/register", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    username: "alice",
    email: "alice@example.com",
    password: "pass123"
  })
})

Login
fetch("http://localhost:8080/auth/login", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    email: "alice@example.com",
    password: "pass123"
  })
})

Create post
fetch("http://localhost:8080/posts", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    title: "Hello",
    body: "First post",
    creator_id: "<USER_ID>"
  })
})

List posts
fetch("http://localhost:8080/posts/list", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    page: 1,
    pageSize: 10,
    sort: { created_at: "descend" },
    filters: {}
  })
})

6. Frontend Examples (axios)

Register
axios.post("http://localhost:8080/auth/register", {
  username: "alice",
  email: "alice@example.com",
  password: "pass123"
})

Login
axios.post("http://localhost:8080/auth/login", {
  email: "alice@example.com",
  password: "pass123"
})

Create post
axios.post("http://localhost:8080/posts", {
  title: "Hello",
  body: "First post",
  creator_id: "<USER_ID>"
})

List posts
axios.post("http://localhost:8080/posts/list", {
  page: 1,
  pageSize: 10,
  sort: { created_at: "descend" },
  filters: {}
})
