FROM node:22-alpine

WORKDIR /app

COPY package.json .
RUN npm install --omit=dev

COPY server.js .

USER node

CMD ["npm", "start"]