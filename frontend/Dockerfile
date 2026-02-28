FROM node:20-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
# Dev server for docker-compose (port 5173)
EXPOSE 5173
CMD ["npm", "run", "dev"]
