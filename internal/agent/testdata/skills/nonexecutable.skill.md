---
name: nonexecutable
description: A pure prose skill (not executable) for backward-compat sanity checks
autoload: false
---

This is a plain skill file without executable: true. The loader should treat
it exactly as before — producing a SkillContent entry with no ExecutableSkillDef.
This file validates that the 4-return LoadSkills call still works for
non-executable skills and the prose is injected into the system prompt normally.
