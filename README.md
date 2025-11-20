# TV Locker Backend

A Go-based backend management system for TV locker service with Supabase database integration, deployable on Vercel.

## Features

- **Device Registration**: Register TV devices with customer information, EMI terms, and term duration
- **Activation Code Generation**: Automatically generates unique activation codes for each EMI term
- **Lock Date Calculation**: Calculates TV lock dates based on term duration (7, 15, or 30 days)
- **Device Activation**: Activate devices using serial number and activation code
- **Remote Locking**: Lock/unlock TVs remotely even when they are turned off
- **Unlock/Uninstall**: API endpoint to unlock or uninstall the app

## Database Setup

1. Go to your Supabase project dashboard
2. Navigate to SQL Editor
3. Run the SQL queries from `schema.sql` file

The schema includes:
- `devices`: Stores device and customer information
- `activation_codes`: Stores generated activation codes for each term
- `lock_dates`: Stores calculated lock dates for each term
- `remote_locks`: Stores remote lock status for each device

## Environment Variables

Create a `.env` file (or set in Vercel dashboard):

```
DATABASE_URL=postgresql://postgres:password@db.project.supabase.co:5432/postgres
PORT=8080
```

For Vercel deployment, add `DATABASE_URL` as an environment variable in your Vercel project settings.

## API Endpoints

### 1. Register Device
**POST** `/api/register`

Register a new device with customer information.

**Request Body:**
```json
{
  "serial_number": "TV123456789",
  "customer_name": "John Doe",
  "phone_number": "+1234567890",
  "emi_term": 9,
  "emi_start_date": "2024-01-01",
  "term_duration": 15
}
```

**Response:**
```json
{
  "success": true,
  "message": "Device registered successfully",
  "device_id": "uuid",
  "terms": [
    {
      "term": 1,
      "lock_date": "2024-01-16",
      "activation_code": "abc12345"
    },
    {
      "term": 2,
      "lock_date": "2024-01-31",
      "activation_code": "def67890"
    },
    {
      "term": 3,
      "lock_date": "2024-02-15",
      "activation_code": "ghi11223"
    }
  ]
}
```

### 2. Activate Device
**POST** `/api/activate`

Activate a device using activation code.

**Request Body:**
```json
{
  "activation_code": "abc12345"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Device activated successfully",
  "terms": [
    {
      "term": 1,
      "lock_date": "2024-01-16"
    },
    {
      "term": 2,
      "lock_date": "2024-01-31"
    },
    {
      "term": 3,
      "lock_date": "2024-02-15"
    }
  ]
}
```

### 3. Check Activation Status
**GET** `/api/check?serial_number=TV123456789`

Check if device is activated and get terms/lock dates.

**Response:**
```json
{
  "success": true,
  "message": "Device is active",
  "terms": [
    {
      "term": 1,
      "lock_date": "2024-01-16"
    },
    {
      "term": 2,
      "lock_date": "2024-01-31"
    },
    {
      "term": 3,
      "lock_date": "2024-02-15"
    }
  ]
}
```

### 4. Set Remote Lock
**POST** `/api/remote-lock`

Lock or unlock TV remotely.

**Request Body:**
```json
{
  "serial_number": "TV123456789",
  "is_locked": true
}
```

**Response:**
```json
{
  "success": true,
  "message": "Remote lock set to true",
  "is_locked": true
}
```

### 5. Check Remote Lock Status
**GET** `/api/check-lock?serial_number=TV123456789`

Check if TV is remotely locked (TV should call this when it turns on).

**Response:**
```json
{
  "is_locked": true
}
```

### 6. Unlock Device
**POST** `/api/unlock`

Unlock device and deactivate it (for uninstall).

**Request Body:**
```json
{
  "serial_number": "TV123456789"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Device unlocked successfully"
}
```

### 7. Health Check
**GET** `/api/health`

Check if the API is running.

**Response:**
```json
{
  "status": "ok"
}
```

## Local Development

1. Install dependencies:
```bash
go mod download
```

2. Set up environment variables in `.env` file

3. Run the server:
```bash
go run main.go
```

## Deployment to Vercel

1. Install Vercel CLI:
```bash
npm i -g vercel
```

2. Deploy:
```bash
vercel
```

3. Set environment variables in Vercel dashboard:
   - Go to your project settings
   - Add `DATABASE_URL` with your Supabase connection string

## How It Works

1. **Registration**: When a device is registered, the system:
   - Creates a device record
   - Generates N activation codes (one per EMI term)
   - Calculates N lock dates based on term duration

2. **Activation**: When TV sends activation request:
   - Validates the activation code
   - Marks the code as used
   - Returns all terms and lock dates
   - TV stores these locally

3. **Remote Locking**: 
   - Admin can set remote lock via API
   - When TV turns on, it calls `/api/check-lock`
   - If locked, TV locks itself

4. **Unlock**: Unlocks device and deactivates it


## Notes

- Term duration must be 7, 15, or 30 days
- Each device gets unique activation codes
- Lock dates are calculated from EMI start date
- Remote locks persist even when TV is off
- TV should periodically check lock status when powered on