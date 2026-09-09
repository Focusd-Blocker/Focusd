document.addEventListener("DOMContentLoaded", () => {
  const quotes = [
    "You installed this blocker for a reason. Don't let your past self down.",
    "Discipline is choosing between what you want now, and what you want most.",
    "Every time you walk away, you get stronger. Walk away.",
    "Is 5 seconds of pixelated dopamine worth the brain fog tomorrow?",
    "You are in control. Your urges do not dictate your actions.",
    "Close the tab. Go drink a glass of water. You've got this.",
  ];
  document.getElementById("quote").innerText =
    `"${quotes[Math.floor(Math.random() * quotes.length)]}"`;

  const breatheText = document.getElementById("breathe-text");
  setInterval(() => {
    if (breatheText.innerText === "Breathe In") {
      breatheText.innerText = "Breathe Out";
    } else {
      breatheText.innerText = "Breathe In";
    }
  }, 4000);

  let timeLeft = 15;
  const countdownEl = document.getElementById("countdown");
  const phase1 = document.getElementById("phase-1");
  const phase2 = document.getElementById("phase-2");

  const timerInterval = setInterval(() => {
    timeLeft--;
    countdownEl.innerText = timeLeft;

    if (timeLeft <= 0) {
      clearInterval(timerInterval);
      phase1.style.display = "none";
      phase2.style.display = "flex";
    }
  }, 1000);

  document.getElementById("btn-close").addEventListener("click", () => {
    browser.runtime.sendMessage({ action: "close_current_tab" });
    // window.location.replace("https://www.google.com");
  });

  document.getElementById("btn-distract").addEventListener("click", () => {
    const safeSites = [
      // "https://neal.fun/", // grab-bag of weird solo interactive toys
      "https://neal.fun/infinite-craft/", // combine elements, oddly hypnotic
      "https://neal.fun/absurd-trolley-problems/",
      "https://www.chess.com/puzzles", // solo puzzles, no need to play a match
      // "https://www.solitr.com/", // solitaire, no account, loads instant
      "https://sudoku.com/",
      "https://www.nytimes.com/games/wordle/index.html",
      "https://www.a4quiz.com/",
      "https://2048game.com/",
      "https://theuselessweb.com/",
    ];
    window.location.replace(
      safeSites[Math.floor(Math.random() * safeSites.length)],
    );
  });
});
