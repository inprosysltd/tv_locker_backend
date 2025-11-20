# Vercel Environment Variable Setup

## Issue
If you see: `Environment Variable "DATABASE_URL" references Secret "database_url", which does not exist`

## Solution

### Step 1: Get Your Supabase Database Connection String

1. Go to your Supabase project dashboard: https://jhfbrrksdhhvzmsydclh.supabase.co
2. Navigate to **Settings** → **Database**
3. Scroll down to **Connection string** section
4. Select **URI** tab (not Session mode)
5. Copy the connection string. It should look like:
   ```
   postgresql://postgres:[YOUR-PASSWORD]@db.jhfbrrksdhhvzmsydclh.supabase.co:5432/postgres
   ```

### Step 2: Set Environment Variable in Vercel

**Option A: Via Vercel Dashboard (Recommended)**

1. Go to your Vercel project dashboard
2. Click on **Settings** → **Environment Variables**
3. Click **Add New**
4. Set:
   - **Key**: `DATABASE_URL`
   - **Value**: Paste your full PostgreSQL connection string from Step 1
   - **Environment**: Select all (Production, Preview, Development)
5. Click **Save**

**Option B: Via Vercel CLI**

```bash
vercel env add DATABASE_URL
# When prompted, paste your connection string
# Select all environments (production, preview, development)
```

### Step 3: Redeploy

After setting the environment variable, redeploy your project:

```bash
vercel --prod
```

Or trigger a new deployment from the Vercel dashboard.

## Important Notes

- The connection string must include your actual password (replace `[YOUR-PASSWORD]` with your database password)
- Make sure you're using the **URI** format, not Session mode
- The connection string should start with `postgresql://` or `postgres://`
- Never commit your connection string to git - always use environment variables

## Verify It's Working

After deployment, test the health endpoint:
```bash
curl https://your-project.vercel.app/api/health
```

If it returns `{"status":"ok"}`, your database connection is working!

