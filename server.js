const express = require('express');
const { Pool } = require('pg');

const app = express();

const pool = new Pool({
  host: '127.0.0.1',
  port: 5432,
  database: process.env.PGDATABASE,
  user: process.env.PGUSER,
  password: process.env.PGPASSWORD
});

app.get('/', async (req, res) => {
  try {
    const result = await pool.query('SELECT NOW() AS now');

    res.send(`
      <!DOCTYPE html>
      <html>
        <head>
          <title>PostgreSQL Test</title>
          <meta name="viewport" content="width=device-width">
          <style>
            body {
              font-family: system-ui;
              display: grid;
              place-items: center;
              min-height: 90vh;
              background: #f5f5f5;
            }
            main {
              background: white;
              padding: 40px;
              border-radius: 12px;
              text-align: center;
            }
            h1 { font-size: 20px; }
            p { color: #555; }
          </style>
        </head>
        <body>
          <main>
            <h1>PostgreSQL is working!</h1>
            <p>${result.rows[0].now.toISOString()}</p>
          </main>
        </body>
      </html>
    `);
  } catch (error) {
    console.error(error);
    res.status(500).send('Database query failed');
  }
});

app.get('/api/now', async (req, res) => {
  try {
    const result = await pool.query('SELECT NOW() AS now');
    res.json(result.rows[0]);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Database query failed' });
  }
});

app.listen(3000, '127.0.0.1', () => {
  console.log('Server listening on localhost:3000');
});