module.exports = {
  apps: [
    {
      name: "ai-token",
      cwd: "/opt/ai-token/current",
      script: "/opt/ai-token/current/deploy/pm2/start.sh",
      interpreter: "/usr/bin/bash",
      exec_mode: "fork",
      instances: 1,
      autorestart: true,
      watch: false,
      min_uptime: "10s",
      max_restarts: 10,
      restart_delay: 3000,
      exp_backoff_restart_delay: 100,
      kill_timeout: 20000,
      merge_logs: true,
      time: true,
      out_file: "/opt/ai-token/.pm2/logs/ai-token-out.log",
      error_file: "/opt/ai-token/.pm2/logs/ai-token-error.log",
    },
  ],
};
