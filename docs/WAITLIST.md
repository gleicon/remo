# Managing the waitlist

Signup is public and unauthenticated. Approval requires the master token.

## How it works

1. Visitor submits email via `POST /waitlist` (or the landing page form).
2. Entry is stored as `pending` in the DB — no access is granted.
3. You review the list, approve or reject with curl.
4. On approve, a user account is created and the token is returned **once** — copy it to the new user.

---

## Submit (public)

```sh
curl -X POST https://remo.yourdomain.tld/waitlist \
  -H 'Content-Type: application/json' \
  -d '{"email": "someone@example.com"}'
# → 201 (no body)
```

Duplicate emails are silently ignored (still returns 201).

---

## Admin endpoints

Set your master token once:

```sh
export REMO_TOKEN=$(cat /etc/remo/master_token)
export REMO_URL=https://remo.yourdomain.tld
```

### List pending

```sh
curl -s -H "Authorization: Bearer $REMO_TOKEN" \
  $REMO_URL/api/admin/waitlist | jq .
```

Output:

```json
[
  { "id": "abc123...", "email": "someone@example.com", "status": "pending", "created_at": "..." }
]
```

### Approve

```sh
curl -s -X POST \
  -H "Authorization: Bearer $REMO_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{}' \
  $REMO_URL/api/admin/waitlist/<id>/approve | jq .
```

Optional: pass a specific username (defaults to email prefix):

```sh
-d '{"username": "alice"}'
```

Output:

```json
{ "username": "alice", "email": "someone@example.com", "token": "..." }
```

Copy the `token` value and send it to the user. It is shown only once.

The user sets it up with:

```sh
remo login --server https://remo.yourdomain.tld --token <token>
```

### Reject

```sh
curl -s -X DELETE \
  -H "Authorization: Bearer $REMO_TOKEN" \
  $REMO_URL/api/admin/waitlist/<id>
# → 204
```

---

## Piping list → approve

Approve all pending entries in one pass:

```sh
curl -s -H "Authorization: Bearer $REMO_TOKEN" \
  $REMO_URL/api/admin/waitlist \
  | jq -r '.[] | select(.status=="pending") | .id' \
  | while read id; do
      result=$(curl -s -X POST \
        -H "Authorization: Bearer $REMO_TOKEN" \
        -H 'Content-Type: application/json' \
        -d '{}' \
        $REMO_URL/api/admin/waitlist/$id/approve)
      email=$(echo $result | jq -r .email)
      token=$(echo $result | jq -r .token)
      echo "$email → $token"
    done
```
