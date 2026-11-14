// Oh My Pi loads this hook file from the plugin, and runs the Pi extension of
// git-multistage with the stage prefix omp.

import gitMultistage from "../../plugins/pi/git-multistage.js";

export default function gitMultistageOmp(pi) {
  gitMultistage(pi, "omp");
}
