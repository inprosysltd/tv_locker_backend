# Deployment Guide

## Supabase Database Setup

1. **Get your Supabase connection string:**
   - Go to your Supabase project dashboard
   - Navigate to Settings → Database
   - Copy the connection string (URI format)
   - Format: `postgresql://postgres:[YOUR-PASSWORD]@db.[PROJECT-REF].supabase.co:5432/postgres`

2. **Run the SQL schema:**
   - Go to SQL Editor in Supabase dashboard
   - Copy and paste the entire contents of `schema.sql`
   - Click "Run" to execute

3. **Verify tables are created:**
   - Go to Table Editor
   - You should see 4 tables:
     - `devices`
     - `activation_codes`
     - `lock_dates`
     - `remote_locks`

## Vercel Deployment

### Option 1: Using Vercel CLI

1. **Install Vercel CLI:**
   ```bash
   npm i -g vercel
   ```

2. **Login to Vercel:**
   ```bash
   vercel login
   ```

3. **Deploy:**
   ```bash
   vercel
   ```

4. **Set Environment Variables:**
   - Go to your project on Vercel dashboard
   - Settings → Environment Variables
   - Add `DATABASE_URL` with your Supabase connection string

### Option 2: Using GitHub Integration

1. Push your code to GitHub
2. Import project in Vercel
3. Add `DATABASE_URL` environment variable
4. Deploy

## Testing the API

After deployment, test the endpoints:

```bash
# Health check
curl https://your-project.vercel.app/api/health

# Register device
curl -X POST https://your-project.vercel.app/api/register \
  -H "Content-Type: application/json" \
  -d '{
    "serial_number": "TV123456789",
    "customer_name": "John Doe",
    "phone_number": "+1234567890",
    "emi_term": 9,
    "emi_start_date": "2024-01-01",
    "term_duration": 15
  }'
```

## Local Development

1. **Install dependencies:**
   ```bash
   go mod download
   ```

2. **Create `.env` file:**
   ```bash
   cp .env.example .env
   # Edit .env with your Supabase connection string
   ```

3. **Run the server:**
   ```bash
   go run main.go
   ```

4. **Test locally:**
   ```bash
   curl http://localhost:8080/api/health
   ```

