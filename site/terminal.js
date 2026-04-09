// terminal.js — Animated terminal playback from real rd demo transcripts
// Each demo is a curated sequence from test/demo/output/*.txt

(function() {
  'use strict';

  const DEMOS = {
    solo: {
      title: 'Solo workflow',
      source: 'https://github.com/campfire-net/ready/blob/main/test/demo/01-solo.sh',
      lines: [
        { type: 'comment', text: '# Initialize a project' },
        { type: 'cmd', text: 'rd init --name myproject' },
        { type: 'output', text: 'initialized myproject' },
        { type: 'output', text: '  campfire: 7b0929f77f95...' },
        { type: 'output', text: '  declarations: 16 operations published' },
        { type: 'blank' },
        { type: 'comment', text: '# Create a work item — bare ID returned' },
        { type: 'cmd', text: 'ITEM=$(rd create "Ship login page" --priority p1 --type task)' },
        { type: 'cmd', text: 'echo $ITEM' },
        { type: 'output', text: 'myproject-e1f' },
        { type: 'blank' },
        { type: 'comment', text: '# What needs attention?' },
        { type: 'cmd', text: 'rd ready' },
        { type: 'output', text: '  myproject-e1f  p1  inbox  3h  Ship login page' },
        { type: 'blank' },
        { type: 'comment', text: '# Claim it' },
        { type: 'cmd', text: 'rd claim $ITEM' },
        { type: 'output', text: 'claimed myproject-e1f' },
        { type: 'blank' },
        { type: 'comment', text: '# Done — close with a reason' },
        { type: 'cmd', text: 'rd done $ITEM --reason "Login page ships with JWT auth"' },
        { type: 'output', text: 'closed myproject-e1f (done)' },
      ]
    },

    team: {
      title: 'Team with invite tokens',
      source: 'https://github.com/campfire-net/ready/blob/main/test/demo/02-team.sh',
      lines: [
        { type: 'comment', text: '# Owner creates project and generates invite' },
        { type: 'cmd', text: 'rd init --name backend' },
        { type: 'output', text: 'initialized backend' },
        { type: 'cmd', text: 'TOKEN=$(rd invite)' },
        { type: 'output', text: 'rdx1_eyJ2IjoxLCJjYW1...  (one-use, expires in 2h)' },
        { type: 'blank' },
        { type: 'comment', text: '# Teammate joins — one command, no key exchange' },
        { type: 'cmd', text: 'rd join $TOKEN' },
        { type: 'output', text: 'joined 00d5716f0154... via invite token (expires in 1h59m)' },
        { type: 'blank' },
        { type: 'comment', text: '# Owner creates work and delegates' },
        { type: 'cmd', text: 'ITEM=$(rd create "Build API" --type task --priority p1)' },
        { type: 'cmd', text: 'rd delegate $ITEM --to <member-identity>' },
        { type: 'output', text: 'delegated backend-776 to 6d2d5a5f4c38...' },
        { type: 'blank' },
        { type: 'comment', text: '# Member sees it — auto-synced, no manual pull' },
        { type: 'cmd', text: 'cd member/ && rd ready' },
        { type: 'output', text: '  backend-776  p1  inbox  3h  Build API' },
        { type: 'blank' },
        { type: 'comment', text: '# Member claims and completes' },
        { type: 'cmd', text: 'rd claim backend-776' },
        { type: 'output', text: 'claimed backend-776' },
        { type: 'cmd', text: 'rd done backend-776 --reason "API complete"' },
        { type: 'output', text: 'closed backend-776 (done)' },
      ]
    },

    gate: {
      title: 'Agent escalation',
      source: 'https://github.com/campfire-net/ready/blob/main/test/demo/06-gate-escalation.sh',
      lines: [
        { type: 'comment', text: '# Agent hits a decision point' },
        { type: 'cmd', text: 'rd gate myapp-dd6 --gate-type design \\' },
        { type: 'cmd-cont', text: '  --description "Option A saves 2ms but breaks caching. Option B is safe."' },
        { type: 'output', text: '{"gate_type":"design","id":"myapp-dd6","status":"waiting"}' },
        { type: 'blank' },
        { type: 'comment', text: '# Item moves to waiting' },
        { type: 'cmd', text: 'rd show myapp-dd6' },
        { type: 'output', text: 'Status:   waiting' },
        { type: 'output', text: 'Waiting on: Option A saves 2ms but breaks caching... (gate)' },
        { type: 'blank' },
        { type: 'comment', text: '# Human sees it from anywhere' },
        { type: 'cmd', text: 'rd gates' },
        { type: 'output', text: '  myapp-dd6  p1  Option A saves 2ms...  Migrate auth layer' },
        { type: 'blank' },
        { type: 'comment', text: '# Human approves — agent continues' },
        { type: 'cmd', text: 'rd approve myapp-dd6 --reason "Use option B. Safety over 2ms."' },
        { type: 'output', text: '{"id":"myapp-dd6","resolution":"approved"}' },
        { type: 'blank' },
        { type: 'comment', text: '# Agent: item is active again' },
        { type: 'cmd', text: 'rd show myapp-dd6' },
        { type: 'output', text: 'Status:   active' },
      ]
    },

    isolation: {
      title: 'Walk-up agent isolation',
      source: 'https://github.com/campfire-net/ready/blob/main/test/demo/11-filesystem-isolation.sh',
      lines: [
        { type: 'comment', text: '# Project directory with two agent worktrees' },
        { type: 'output', text: 'myproject/' },
        { type: 'output', text: '  .campfire/root              \u2190 project (shared)' },
        { type: 'output', text: '  .cf/identity.json           \u2190 owner' },
        { type: 'output', text: '  worktree-a/.cf/identity.json  \u2190 agent A' },
        { type: 'output', text: '  worktree-b/.cf/identity.json  \u2190 agent B' },
        { type: 'blank' },
        { type: 'comment', text: '# Agent A: cd and run. Walk-up handles everything.' },
        { type: 'cmd', text: 'cd myproject/worktree-a' },
        { type: 'cmd', text: 'rd ready' },
        { type: 'output', text: '  myproject-460  p1  inbox  3h  Implement login' },
        { type: 'output', text: '  myproject-b22  p1  inbox  3h  Write tests' },
        { type: 'blank' },
        { type: 'comment', text: '# Agent A claims one item' },
        { type: 'cmd', text: 'ITEM=$(rd ready | head -1)' },
        { type: 'cmd', text: 'rd claim $ITEM' },
        { type: 'output', text: 'claimed myproject-460' },
        { type: 'blank' },
        { type: 'comment', text: '# Agent B: same project, different identity, zero config' },
        { type: 'cmd', text: 'cd myproject/worktree-b' },
        { type: 'cmd', text: 'rd claim myproject-b22' },
        { type: 'output', text: 'claimed myproject-b22' },
        { type: 'blank' },
        { type: 'comment', text: '# Owner sees both claims with different agent identities' },
        { type: 'cmd', text: 'rd list --all --json | jq ".[].by"' },
        { type: 'output', text: '"a3f2...agent-a"' },
        { type: 'output', text: '"7c1e...agent-b"' },
      ]
    }
  };

  // Timing constants
  const CHAR_DELAY = 18;       // ms per character for commands
  const LINE_PAUSE = 120;      // ms pause after each line
  const CMD_PAUSE = 400;       // ms pause after command before output
  const SECTION_PAUSE = 600;   // ms pause at blank lines
  const RESTART_DELAY = 3000;  // ms before looping

  class TerminalPlayer {
    constructor(el, demo) {
      this.container = el;
      this.demo = demo;
      this.lines = demo.lines;
      this.currentLine = 0;
      this.playing = false;
      this.abortController = null;
      this.render();
    }

    render() {
      this.container.innerHTML = '';

      // Header bar
      const header = document.createElement('div');
      header.className = 'term-header';

      const dots = document.createElement('div');
      dots.className = 'term-dots';
      dots.innerHTML = '<span></span><span></span><span></span>';

      const title = document.createElement('span');
      title.className = 'term-title';
      title.textContent = this.demo.title;

      const controls = document.createElement('div');
      controls.className = 'term-controls';

      this.playBtn = document.createElement('button');
      this.playBtn.className = 'term-play';
      this.playBtn.textContent = '\u25B6';
      this.playBtn.title = 'Play';
      this.playBtn.addEventListener('click', () => this.toggle());

      const sourceLink = document.createElement('a');
      sourceLink.href = this.demo.source;
      sourceLink.className = 'term-source';
      sourceLink.textContent = 'source';
      sourceLink.target = '_blank';
      sourceLink.rel = 'noopener';

      controls.appendChild(this.playBtn);
      controls.appendChild(sourceLink);

      header.appendChild(dots);
      header.appendChild(title);
      header.appendChild(controls);

      // Terminal body
      this.body = document.createElement('div');
      this.body.className = 'term-body';

      // Prompt (cursor)
      this.prompt = document.createElement('div');
      this.prompt.className = 'term-prompt';
      this.prompt.innerHTML = '<span class="term-ps1">$</span> <span class="term-cursor"></span>';

      this.body.appendChild(this.prompt);

      this.container.appendChild(header);
      this.container.appendChild(this.body);
    }

    toggle() {
      if (this.playing) {
        this.stop();
      } else {
        this.play();
      }
    }

    async play() {
      this.playing = true;
      this.playBtn.textContent = '\u275A\u275A';
      this.playBtn.title = 'Pause';
      this.abortController = new AbortController();

      // Clear previous output
      this.body.innerHTML = '';
      this.body.appendChild(this.prompt);
      this.currentLine = 0;

      try {
        await this.runLines();
      } catch (e) {
        if (e.name !== 'AbortError') throw e;
      }

      if (this.playing) {
        // Loop
        await this.sleep(RESTART_DELAY);
        if (this.playing) this.play();
      }
    }

    stop() {
      this.playing = false;
      this.playBtn.textContent = '\u25B6';
      this.playBtn.title = 'Play';
      if (this.abortController) this.abortController.abort();
    }

    async runLines() {
      for (let i = 0; i < this.lines.length; i++) {
        this.checkAbort();
        const line = this.lines[i];

        if (line.type === 'blank') {
          this.addBlank();
          await this.sleep(SECTION_PAUSE);
          continue;
        }

        if (line.type === 'comment') {
          this.addLine('term-comment', line.text);
          await this.sleep(LINE_PAUSE);
          continue;
        }

        if (line.type === 'cmd' || line.type === 'cmd-cont') {
          await this.typeCommand(line.text, line.type === 'cmd-cont');
          await this.sleep(CMD_PAUSE);
          continue;
        }

        if (line.type === 'output') {
          this.addLine('term-output', line.text);
          await this.sleep(LINE_PAUSE);
          continue;
        }
      }
    }

    async typeCommand(text, isContinuation) {
      // Hide prompt during typing
      this.prompt.style.display = 'none';

      const lineEl = document.createElement('div');
      lineEl.className = 'term-line term-cmd';

      if (!isContinuation) {
        const ps1 = document.createElement('span');
        ps1.className = 'term-ps1';
        ps1.textContent = '$ ';
        lineEl.appendChild(ps1);
      } else {
        // Indent continuation
        const indent = document.createElement('span');
        indent.className = 'term-ps1';
        indent.textContent = '  ';
        lineEl.appendChild(indent);
      }

      const textSpan = document.createElement('span');
      lineEl.appendChild(textSpan);

      const cursor = document.createElement('span');
      cursor.className = 'term-cursor';
      lineEl.appendChild(cursor);

      // Insert before prompt
      this.body.insertBefore(lineEl, this.prompt);
      this.scrollToBottom();

      // Type characters
      for (let j = 0; j < text.length; j++) {
        this.checkAbort();
        textSpan.textContent += text[j];
        this.scrollToBottom();
        await this.sleep(CHAR_DELAY);
      }

      // Remove cursor from this line
      cursor.remove();

      // Show prompt again
      this.prompt.style.display = '';
      this.scrollToBottom();
    }

    addLine(className, text) {
      const lineEl = document.createElement('div');
      lineEl.className = 'term-line ' + className;
      lineEl.textContent = text;
      this.body.insertBefore(lineEl, this.prompt);
      this.scrollToBottom();
    }

    addBlank() {
      const lineEl = document.createElement('div');
      lineEl.className = 'term-line term-blank';
      lineEl.innerHTML = '&nbsp;';
      this.body.insertBefore(lineEl, this.prompt);
    }

    scrollToBottom() {
      this.body.scrollTop = this.body.scrollHeight;
    }

    sleep(ms) {
      return new Promise((resolve, reject) => {
        const id = setTimeout(resolve, ms);
        if (this.abortController) {
          this.abortController.signal.addEventListener('abort', () => {
            clearTimeout(id);
            reject(new DOMException('Aborted', 'AbortError'));
          });
        }
      });
    }

    checkAbort() {
      if (this.abortController && this.abortController.signal.aborted) {
        throw new DOMException('Aborted', 'AbortError');
      }
    }
  }

  // Auto-init: find all [data-terminal-demo] elements
  document.addEventListener('DOMContentLoaded', function() {
    document.querySelectorAll('[data-terminal-demo]').forEach(function(el) {
      var demoName = el.getAttribute('data-terminal-demo');
      if (DEMOS[demoName]) {
        new TerminalPlayer(el, DEMOS[demoName]);
      }
    });

    // Auto-play the first visible terminal
    var first = document.querySelector('[data-terminal-demo]');
    if (first && first._player) {
      first._player.play();
    }
  });
})();
