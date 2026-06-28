# Buzzheavier API

API reference for interacting with buzzheavier.com programmatically.

**Base URLs**
- Upload: `https://w.buzzheavier.com`
- REST API: `https://buzzheavier.com/api`

**Authentication**

All authenticated endpoints require:
```
Authorization: Bearer YOUR_TOKEN
```

---

## File Upload

Upload uses a plain HTTP `PUT` with the file body. No multipart form encoding.

### Anonymous upload

```
PUT https://w.buzzheavier.com/{name}
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | path | Yes | File name, max 500 chars |

```bash
curl -#o - -T "sample.mp4" "https://w.buzzheavier.com/sample.mp4" | cat
```

---

### Upload to a user folder

```
PUT https://w.buzzheavier.com/{parentId}/{name}
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `parentId` | path | Yes | ID of the destination folder |
| `name` | path | Yes | File name, max 500 chars |

```bash
curl -#o - -T "sample.mp4" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  "https://w.buzzheavier.com/{parentId}/sample.mp4" | cat
```

---

### Upload to a specific storage location

```
PUT https://w.buzzheavier.com/{name}?locationId={locationId}
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | path | Yes | File name, max 500 chars |
| `locationId` | query | No | Storage location ID |

```bash
curl -#o - -T "sample.mp4" \
  "https://w.buzzheavier.com/sample.mp4?locationId={locationId}" | cat
```

---

### Upload with a note

```
PUT https://w.buzzheavier.com/{name}?note={base64(note)}
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | path | Yes | File name, max 500 chars |
| `note` | query | No | Base64-encoded note text, max 500 chars (shown under the download link) |

```bash
curl -#o - -T "sample.mp4" \
  "https://w.buzzheavier.com/sample.mp4?note=$(echo -n "your note" | base64)" | cat
```

---

## Public APIs

### Get storage locations

```
GET https://buzzheavier.com/api/locations
```

Returns the storage locations available to the authenticated account.

---

## Account

### Get account information

```
GET https://buzzheavier.com/api/account
```

Returns the current authenticated account's information.

---

## File Manager

### Get root directory

```
GET https://buzzheavier.com/api/fs
```

Returns the contents of the root directory.

---

### Get directory contents

```
GET https://buzzheavier.com/api/fs/{directoryId}
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `directoryId` | path | Yes | ID of the directory to list |

---

### Create directory

```
POST https://buzzheavier.com/api/fs/{parentDirectoryId}
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `parentDirectoryId` | path | Yes | ID of the parent directory. To create under root, get the root `directoryId` first. |

**Request body**

```json
{
  "name": "string"
}
```

---

### Rename directory

```
PATCH https://buzzheavier.com/api/fs/{directoryId}
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `directoryId` | path | Yes | ID of the directory to rename |

**Request body**

```json
{
  "name": "string"
}
```

---

### Move directory

```
PATCH https://buzzheavier.com/api/fs/{directoryId}
```

> **Note:** The official docs say `PUT`, but the server returns `405 Method Not Allowed` for `PUT`. Use `PATCH`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `directoryId` | path | Yes | ID of the directory to move |

**Request body**

```json
{
  "parentId": "string"
}
```

---

### Rename file

```
PATCH https://buzzheavier.com/api/fs/{fileId}
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `fileId` | path | Yes | ID of the file to rename |

**Request body**

```json
{
  "name": "string"
}
```

---

### Move file

```
PATCH https://buzzheavier.com/api/fs/{fileId}
```

> **Note:** The official docs say `PUT`, but the server returns `405 Method Not Allowed` for `PUT`. Use `PATCH`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `fileId` | path | Yes | ID of the file to move |

**Request body**

```json
{
  "parentId": "string"
}
```

---

### Edit file note

```
PATCH https://buzzheavier.com/api/fs/{fileId}
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `fileId` | path | Yes | ID of the file |

**Request body**

```json
{
  "note": "string"
}
```

---

### Delete directory

```
DELETE https://buzzheavier.com/api/fs/{directoryId}
```

Deletes the specified directory and all its contents recursively.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `directoryId` | path | Yes | ID of the directory to delete |

---

### Delete file

```
DELETE https://buzzheavier.com/api/fs/{fileId}
```

> **Note:** Not officially documented — uses the same endpoint as directory deletion.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `fileId` | path | Yes | ID of the file to delete |
