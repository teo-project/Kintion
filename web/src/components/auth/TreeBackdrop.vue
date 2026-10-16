<script setup lang="ts">
// Фон входа — «растущее дерево» (F-90): камера бежит вверх за веткой, нижние
// карточки уменьшаются и уходят за экран, сверху прорастают новые. Чистый CSS,
// постоянный DOM (11 карточек, 10 линий), без скриптов. Геометрия и формулы —
// docs/concepts/interface-style.md; цвета — токены --kin-tree-*.
</script>

<template>
  <svg
    class="tree"
    viewBox="520 0 600 640"
    preserveAspectRatio="xMidYMax slice"
    aria-hidden="true"
    focusable="false"
  >
    <defs>
      <g id="kin-tree-person">
        <rect
          class="card"
          x="-26"
          y="-34"
          width="52"
          height="68"
          rx="12"
        />
        <circle
          class="avatar"
          cx="0"
          cy="-10"
          r="9"
        />
        <rect
          class="avatar"
          x="-15"
          y="4"
          width="30"
          height="9"
          rx="4.5"
        />
        <rect
          class="avatar half"
          x="-14"
          y="21"
          width="28"
          height="4"
          rx="2"
        />
      </g>
    </defs>

    <g class="tg tg0">
      <g transform="translate(-90 0)">
        <g class="tpop tpl"><use href="#kin-tree-person" /></g>
      </g>
      <g transform="translate(270 0)">
        <g class="tpop tpl"><use href="#kin-tree-person" /></g>
      </g>
    </g>
    <g class="tg tg1">
      <path
        class="tl tll"
        pathLength="1"
        d="M 90 -34 V -80.2 H -115.4 V -126.4"
      />
      <path
        class="tl tll"
        pathLength="1"
        d="M 90 -34 V -80.2 H 346.2 V -126.4"
      />
      <g transform="translate(90 0)">
        <g class="tpop tpm"><use href="#kin-tree-person" /></g>
      </g>
    </g>
    <g class="tg tg2">
      <path
        class="tl tle"
        pathLength="1"
        d="M -90 -34 V -80.2 H 115.4 V -126.4"
      />
      <g transform="translate(-90 0)"><use href="#kin-tree-person" /></g>
      <g transform="translate(270 0)"><use href="#kin-tree-person" /></g>
    </g>
    <g class="tg tg3">
      <path
        class="tl"
        pathLength="1"
        d="M 90 -34 V -80.2 H -115.4 V -126.4"
      />
      <path
        class="tl"
        pathLength="1"
        d="M 90 -34 V -80.2 H 346.2 V -126.4"
      />
      <g transform="translate(90 0)"><use href="#kin-tree-person" /></g>
    </g>
    <g class="tg tg4">
      <path
        class="tl"
        pathLength="1"
        d="M -90 -34 V -80.2 H 115.4 V -126.4"
      />
      <g transform="translate(-90 0)"><use href="#kin-tree-person" /></g>
      <g transform="translate(270 0)"><use href="#kin-tree-person" /></g>
    </g>
    <g class="tg tg5">
      <path
        class="tl"
        pathLength="1"
        d="M 90 -34 V -80.2 H -115.4 V -126.4"
      />
      <path
        class="tl"
        pathLength="1"
        d="M 90 -34 V -80.2 H 346.2 V -126.4"
      />
      <g transform="translate(90 0)"><use href="#kin-tree-person" /></g>
    </g>
    <g class="tg tg6">
      <path
        class="tl"
        pathLength="1"
        d="M -90 -34 V -80.2 H 115.4 V -126.4"
      />
      <g transform="translate(-90 0)"><use href="#kin-tree-person" /></g>
      <g transform="translate(270 0)"><use href="#kin-tree-person" /></g>
    </g>
  </svg>
</template>

<style scoped>
.tree {
  display: block;
  width: 100%;
  height: 100%;
  pointer-events: none;
}

.card {
  fill: var(--kin-tree-card);
  stroke: var(--kin-tree-card-line);
  stroke-width: 1.5;
}

.avatar {
  fill: var(--kin-tree-avatar);
}

.avatar.half {
  opacity: 0.5;
}

/* цикл 20 с = два уровня; группы движутся на два уровня вниз и уменьшаются */
.tg {
  animation-duration: 20s;
  animation-timing-function: linear;
  animation-iteration-count: infinite;
}

.tg0 { animation-name: tg0; transform: translate(720px, -388.1px) scale(2.107); opacity: 0; }
.tg1 { animation-name: tg1; transform: translate(720px, -108.7px) scale(1.644); opacity: 0; }
.tg2 { animation-name: tg2; transform: translate(720px, 109.3px) scale(1.282); opacity: 1; }
.tg3 { animation-name: tg3; transform: translate(720px, 279.3px) scale(1); opacity: 1; }
.tg4 { animation-name: tg4; transform: translate(720px, 411.9px) scale(0.78); opacity: 1; }
.tg5 { animation-name: tg5; transform: translate(720px, 515.3px) scale(0.608); opacity: 0.72; }
.tg6 { animation-name: tg6; transform: translate(720px, 596px) scale(0.475); opacity: 0.17; }

.tl {
  fill: none;
  stroke: var(--kin-tree-line);
  stroke-width: 2.2;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-dasharray: 1;
}

.tle,
.tll {
  stroke-dashoffset: 1;
  animation-duration: 20s;
  animation-timing-function: linear;
  animation-iteration-count: infinite;
  animation-fill-mode: both;
}

.tle { animation-name: tle; }
.tll { animation-name: tll; }

.tpop {
  opacity: 0;
  transform-box: fill-box;
  transform-origin: center;
  animation-duration: 20s;
  animation-timing-function: ease-out;
  animation-iteration-count: infinite;
  animation-fill-mode: both;
}

.tpm { animation-name: tpm; }
.tpl { animation-name: tpl; }

@keyframes tle { 0% { stroke-dashoffset: 1; } 47% { stroke-dashoffset: 0; } 100% { stroke-dashoffset: 0; } }
@keyframes tll { 0%, 50% { stroke-dashoffset: 1; } 97% { stroke-dashoffset: 0; } 100% { stroke-dashoffset: 0; } }
@keyframes tpm { 0%, 44% { opacity: 0; transform: scale(0.6); } 50%, 100% { opacity: 1; transform: scale(1); } }
@keyframes tpl { 0%, 94% { opacity: 0; transform: scale(0.6); } 99%, 100% { opacity: 1; transform: scale(1); } }
@keyframes tg0 { 0% { transform: translate(720px, -388.1px) scale(2.107); opacity: 0; } 10% { transform: translate(720px, -326.5px) scale(2.005); opacity: 0; } 20% { transform: translate(720px, -267.9px) scale(1.908); opacity: 0; } 30% { transform: translate(720px, -212.2px) scale(1.815); opacity: 0; } 40% { transform: translate(720px, -159.1px) scale(1.727); opacity: 0; } 50% { transform: translate(720px, -108.7px) scale(1.644); opacity: 0; } 60% { transform: translate(720px, -60.7px) scale(1.564); opacity: 0; } 70% { transform: translate(720px, -15px) scale(1.488); opacity: 0; } 80% { transform: translate(720px, 28.5px) scale(1.416); opacity: 0; } 90% { transform: translate(720px, 69.9px) scale(1.347); opacity: 0.33; } 100% { transform: translate(720px, 109.3px) scale(1.282); opacity: 1; } }
@keyframes tg1 { 0% { transform: translate(720px, -108.7px) scale(1.644); opacity: 0; } 10% { transform: translate(720px, -60.7px) scale(1.564); opacity: 0; } 20% { transform: translate(720px, -15px) scale(1.488); opacity: 0; } 30% { transform: translate(720px, 28.5px) scale(1.416); opacity: 0; } 40% { transform: translate(720px, 69.9px) scale(1.347); opacity: 0.33; } 50% { transform: translate(720px, 109.3px) scale(1.282); opacity: 1; } 60% { transform: translate(720px, 146.7px) scale(1.22); opacity: 1; } 70% { transform: translate(720px, 182.4px) scale(1.161); opacity: 1; } 80% { transform: translate(720px, 216.3px) scale(1.104); opacity: 1; } 90% { transform: translate(720px, 248.6px) scale(1.051); opacity: 1; } 100% { transform: translate(720px, 279.3px) scale(1); opacity: 1; } }
@keyframes tg2 { 0% { transform: translate(720px, 109.3px) scale(1.282); opacity: 1; } 10% { transform: translate(720px, 146.7px) scale(1.22); opacity: 1; } 20% { transform: translate(720px, 182.4px) scale(1.161); opacity: 1; } 30% { transform: translate(720px, 216.3px) scale(1.104); opacity: 1; } 40% { transform: translate(720px, 248.6px) scale(1.051); opacity: 1; } 50% { transform: translate(720px, 279.3px) scale(1); opacity: 1; } 60% { transform: translate(720px, 308.5px) scale(0.952); opacity: 1; } 70% { transform: translate(720px, 336.3px) scale(0.905); opacity: 1; } 80% { transform: translate(720px, 362.7px) scale(0.862); opacity: 1; } 90% { transform: translate(720px, 387.9px) scale(0.82); opacity: 1; } 100% { transform: translate(720px, 411.9px) scale(0.78); opacity: 1; } }
@keyframes tg3 { 0% { transform: translate(720px, 279.3px) scale(1); opacity: 1; } 10% { transform: translate(720px, 308.5px) scale(0.952); opacity: 1; } 20% { transform: translate(720px, 336.3px) scale(0.905); opacity: 1; } 30% { transform: translate(720px, 362.7px) scale(0.862); opacity: 1; } 40% { transform: translate(720px, 387.9px) scale(0.82); opacity: 1; } 50% { transform: translate(720px, 411.9px) scale(0.78); opacity: 1; } 60% { transform: translate(720px, 434.7px) scale(0.742); opacity: 1; } 70% { transform: translate(720px, 456.3px) scale(0.706); opacity: 1; } 80% { transform: translate(720px, 477px) scale(0.672); opacity: 0.94; } 90% { transform: translate(720px, 496.6px) scale(0.639); opacity: 0.83; } 100% { transform: translate(720px, 515.3px) scale(0.608); opacity: 0.72; } }
@keyframes tg4 { 0% { transform: translate(720px, 411.9px) scale(0.78); opacity: 1; } 10% { transform: translate(720px, 434.7px) scale(0.742); opacity: 1; } 20% { transform: translate(720px, 456.3px) scale(0.706); opacity: 1; } 30% { transform: translate(720px, 477px) scale(0.672); opacity: 0.94; } 40% { transform: translate(720px, 496.6px) scale(0.639); opacity: 0.83; } 50% { transform: translate(720px, 515.3px) scale(0.608); opacity: 0.72; } 60% { transform: translate(720px, 533.1px) scale(0.579); opacity: 0.61; } 70% { transform: translate(720px, 550px) scale(0.551); opacity: 0.5; } 80% { transform: translate(720px, 566.1px) scale(0.524); opacity: 0.39; } 90% { transform: translate(720px, 581.4px) scale(0.499); opacity: 0.28; } 100% { transform: translate(720px, 596px) scale(0.475); opacity: 0.17; } }
@keyframes tg5 { 0% { transform: translate(720px, 515.3px) scale(0.608); opacity: 0.72; } 10% { transform: translate(720px, 533.1px) scale(0.579); opacity: 0.61; } 20% { transform: translate(720px, 550px) scale(0.551); opacity: 0.5; } 30% { transform: translate(720px, 566.1px) scale(0.524); opacity: 0.39; } 40% { transform: translate(720px, 581.4px) scale(0.499); opacity: 0.28; } 50% { transform: translate(720px, 596px) scale(0.475); opacity: 0.17; } 60% { transform: translate(720px, 609.8px) scale(0.452); opacity: 0.06; } 70% { transform: translate(720px, 623px) scale(0.43); opacity: 0; } 80% { transform: translate(720px, 635.6px) scale(0.409); opacity: 0; } 90% { transform: translate(720px, 647.5px) scale(0.389); opacity: 0; } 100% { transform: translate(720px, 658.9px) scale(0.37); opacity: 0; } }
@keyframes tg6 { 0% { transform: translate(720px, 596px) scale(0.475); opacity: 0.17; } 10% { transform: translate(720px, 609.8px) scale(0.452); opacity: 0.06; } 20% { transform: translate(720px, 623px) scale(0.43); opacity: 0; } 30% { transform: translate(720px, 635.6px) scale(0.409); opacity: 0; } 40% { transform: translate(720px, 647.5px) scale(0.389); opacity: 0; } 50% { transform: translate(720px, 658.9px) scale(0.37); opacity: 0; } 60% { transform: translate(720px, 669.7px) scale(0.352); opacity: 0; } 70% { transform: translate(720px, 680px) scale(0.335); opacity: 0; } 80% { transform: translate(720px, 689.8px) scale(0.319); opacity: 0; } 90% { transform: translate(720px, 699.1px) scale(0.303); opacity: 0; } 100% { transform: translate(720px, 708px) scale(0.289); opacity: 0; } }

@media (prefers-reduced-motion: reduce) {
  .tg,
  .tle,
  .tll,
  .tpop {
    animation: none;
  }

  .tle,
  .tll {
    stroke-dashoffset: 0;
  }

  .tpop {
    opacity: 1;
  }
}
</style>
