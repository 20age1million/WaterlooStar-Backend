WaterlooStar BackEnd API (Quick)

This is a short version for quick frontend integration.
For more detail, see API_FULL.md.

1. Base

Base URL
http://localhost:8080

Headers
Content-Type: application/json

2. Health

Purpose
Check server status.

Endpoint
GET /health

Response 200
ok

3. Auth

3.1 Register

Endpoint
POST /auth/register

Request body
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "pass123"
}

Responses
200:
{"status":"ok"}
409:
{"status":"error","error":"duplicate username"}
{"status":"error","error":"duplicate email"}
400:
{"status":"error","error":"..."}

3.2 Login

Endpoint
POST /auth/login

Request body
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "pass123"
}

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

Endpoint
POST /auth/send-code

Request body
{
  "email":"alice@example.com"
}

Responses
200:
{"status":"ok"}
401:
unauthorized

3.4 Verify Code

Endpoint
POST /auth/verify-code

Request body
{
  "email":"alice@example.com",
  "code":"123456"
}

Responses
200:
{"status":"ok"}
401:
unauthorized

4. Posts

4.1 List Posts (simple)

Endpoint
GET /posts

Query params
limit (default 20)
offset (default 0)

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

Endpoint
POST /posts

Request body
{
  "title":"Hello",
  "body":"First post",
  "creator_id":"<USER_ID>",
  "images":[]
}

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

Notes
- Verification codes are stored in memory and expire after 10 minutes.
- time_from/time_to must be RFC3339 format.
