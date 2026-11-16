# gighub
Gighub

`task dev` to start development server. Open http://localhost:7331 in your browser.

> **Documentation**: See docs/BUSINESS_RULES.md for detailed permission logic and domain rules.

## Roadmap

### ✅ Completed
- **Core Infrastructure**
  - [x] Project Setup (Go, Chi, Templ, Tailwind, SQLite, sqlc)
  - [x] Database Migration & Query Generation
  - [x] Hot Reloading (Air & Templ proxy)
- **Authentication & Users**
  - [x] Sign up with Email/Password
  - [x] Login with Email/Password
  - [x] Social Login (Google OAuth)
  - [x] Email Verification (SMTP)
  - [x] Session Management (SCS & Gorilla)
- **Security**
  - [x] CSRF Protection (Nosurf)
  - [x] Password Hashing (bcrypt)
  - [x] Secure Headers & Middleware
- **Features**
  - [x] Guestbook (Read/Write)
  - [x] User Account Dashboard
  - [x] Admin Interface (sqliteadmin)
  - [x] Static Pages (Privacy Policy, Terms of Service)

### 🚧 In Progress / Planned
- [ ] Password Reset Flow
- [ ] User Profile Editing (Name, Avatar upload)
- [ ] Flash Messages (Success/Error notifications)
- [ ] Guestbook Pagination & History
- [ ] Form Validation Feedback (UI improvements)
- [ ] Unit & Integration Tests

### Artist & Performer Ecosystem
- [ ] Artist Listing Page (Support for Solo, Bands, DJs)
- [ ] Artist Profiles (Genres, Instruments, Location)
- [ ] Band Management (Members, Solo vs Band)
- [ ] Artist Ratings & Reviews

### Gig Management
  - [ ] Public Landing Page (Show upcoming gigs to visitors)
- [ ] Gig Listings (Upcoming & Historical)
- [ ] Linking Artists to Gigs
- [ ] Gig Ratings & Reviews

# Business Rules & Logic

## 1. User Roles & Permissions

### Verified Users
- A user is considered **Verified** if they have confirmed their email address (via magic link) or signed up via a trusted Social Provider (e.g., Google).
- **Capabilities**:
  - Can create new **Artist** profiles.
  - Can create new **Gig** listings.

### Content Ownership & Editing
- **Ownership**: The user who creates an entity (Artist, Band, Gig) is recorded as its **Owner**.
- **Editing Rights**:
  - **Owner**: Can edit the entities they created.
  - **Admin**: Can edit or delete *any* entity in the system.
  - **Other Users**: Read-only access to entities they do not own.

## 2. Artists & Bands
- An **Artist** entity represents a performer.
- An Artist can be a **Solo** act, a **Band**, or a **DJ**.
- Artists have specific metadata: Genres, Instruments, Location.
- **Constraint**: An Artist name must be unique within a specific location? (TBD)

## 3. Gigs
- **Listing**: Gigs can be **Upcoming** (future date) or **Historical** (past date).
- **Association**: A Gig can be linked to artists who performed at that event.
- **Reviews**:
  - Users can rate and review Gigs they attended (Historical).
  - Users can rate and review Artists.

## 4. Media & Content Strategy
- **Lean Backend Principle**: The application server and database will not store binary media files.
- **Images (Avatars, Posters)**: Stored in external Object Storage (e.g., AWS S3, Cloudflare R2). The database stores the public URL.
- **Audio & Video**: Handled via embedding. Users provide links to external platforms (YouTube, SoundCloud, Spotify, Bandcamp), and the application renders the appropriate embed player.