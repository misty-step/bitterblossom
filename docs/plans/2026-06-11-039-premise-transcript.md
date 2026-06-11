# Premise transcript (sanitized excerpts) — submission loop direction

Operator voice dictation, 2026-06-11 session, sanitized for repo storage.
Key passages preserved verbatim where load-bearing; filler removed.

On the actual scarce resource:

> "The scarce resource is actually tokens because I am not personally
> reviewing this code. [...] I am setting up a system that should all
> but guarantee that we are working on things that are important, that
> are fleshed out, and that actually work and add value and don't break
> anything."

On the loop:

> "We are trying to take the work that gets done and [...] create some
> kind of trigger, a reflex that takes work when it is complete and runs
> it through this storm of adversarial reviews and quality checks. QA
> agents should be spun up, code reviewing agents should be spun up, red
> team agents should be spun up, product owner agents should be spun up.
> We need to just attack the work and create all of this feedback that
> actually is useful for the implementing agent."

On termination:

> "If you ask an agent to be an adversarial reviewer, it will often just
> say that it is not good enough. [...] You don't want to get hung up
> forever addressing nitpicks, especially since frequently they will
> nitpick one thing and you fix it and then they nitpick the opposite
> thing. [...] There needs to be some sanity checking so that we don't
> just get endlessly needled into submission."

On the report contract:

> "Everything in the report is either something that needs to be done
> before the merge, that needs to be logged to be done later, or is
> something that is not even worth logging."

On the loop's exit:

> "This loops until some standard of quality is achieved, until the
> report does not contain anything that is blocking to the merge."

On the PR as primitive:

> "I am not convinced that the pull request is necessarily the best
> primitive for this, because the pull request by nature is a GitHub
> thing and not a Git thing. [...] The fundamental purpose of GitHub is
> that it is where your remotes live. [Agents] don't necessarily need to
> query GitHub for pull requests — they can just look at diffs;
> everything is just SHAs."

On the fallback:

> "If we wind up spinning our wheels on this, we can just put a pin in
> it and open a pull request [...] the PR waits for CI to complete, and
> once CI completes, that triggers the storm of adversarial review
> agents [...] and this loops until some standard of quality is
> achieved."

Design assumptions ratified for brainstorming and carried into the shape:
humans neither read nor write code; humans manage agent planes, reflex
definitions, and dispatch sessions; agents coordinate across substrates;
git remains; GitHub is not assumed. jj migration is anticipated ("we're
also moving off of git and onto jj — it uses git under the hood").

v1 trigger decision (AskUserQuestion, this session): dispatch-driven
first — the implementing agent fires the storm; reflex ingress follows
once the plane has a durable home.
