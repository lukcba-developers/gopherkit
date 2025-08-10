-- Init script for User API database
-- This script sets up the initial database structure and data

-- Ensure the database exists (handled by docker-compose environment)

-- Create extensions if needed
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Create indexes for performance (GORM will create tables automatically)
-- These will be created after tables exist

-- Create function to update timestamps
CREATE OR REPLACE FUNCTION update_modified_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create sample tenants for demo purposes
INSERT INTO "tenants" (id, name, created_at, updated_at) VALUES 
    ('tenant-demo-1', 'Demo Tennis Club', NOW(), NOW()),
    ('tenant-demo-2', 'Demo Football Club', NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

-- Note: The actual tables (users, user_stats) will be created automatically
-- by GORM auto-migration when the application starts