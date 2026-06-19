{{Shortcut|RS:PRICES|RS:REALTIME}}
We've partnered with [https://runelite.net/ RuneLite] to bring '''real-time Grand Exchange prices''' to the wiki. You can check an item's current prices by clicking the '''View Real-time Prices''' on any item article. We plan to add historical real-time price graphs in the near future.

We're also sharing the real-time pricing APIs with the community, in the hope that people will use the data for interesting projects and community-facing tools.

Please be aware that this is a fairly new project, and the endpoints may have downtime, and the prices may not be 100% accurate.

==Discord channel==
If you're planning to use this for some sort of real-time pipeline, please come talk to us in the #api-discussion channel on our [http://discord.gg/runescapewiki Discord] so we can keep you informed about future changes and maintenance.

If you react to the message in the #welcome, you'll be able to see the #api-discussion channel.

==Acceptable use policy==
Within reason, we want people to use these APIs as much as they need to build cool projects and tools. We do not explicitly ratelimit any of the endpoints, and we do our best to cache the responses at multiple levels.

However, we reserve the right to limit access to anyone, if their usage is so frequent that it threatens the stability of the entire API. We don't know where that line is right now, but for Grand Exchange prices, it would probably have to be multiple large queries per second for a sustained period. If we end up blocking your tool, feel free to reach out on [https://discord.gg/runescapewiki Discord] and we'll see if there's a better solution for what you're doing.

===Please set a descriptive User-Agent!===
This is the only thing we ask! If you're using automated tooling to scrape the wiki's APIs, please set a User-Agent that describes what you're using it for, and if you're willing, some sort of contact info (like an email or Discord).

This helps us understand what people are using the APIs for, and helps us reach out in advance if there are any breaking changes coming, or your usage is problematic (or can be improved using bulk modules).

We currently pre-emptively block the following user-agents, and may add more:
* python-requests
* Python-urllib
* Apache-HttpClient
* RestSharp
* Java/{version}
* curl/{version}

See [https://stackoverflow.com/a/10606260 here] for how to change the User-Agent in python-requests, which seems to be the dominant tool people are using

An awesome example of a User-Agent would be something like "volume_tracker - @ThisIsMyUsername on Discord".

Note this does '''not at all''' mean you can't use the python-requests library or similar, but we just ask that you set a user-agent in your code.

==Routes==
* API endpoint: <code>prices.runescape.wiki/api/v1/osrs</code>
* Deadman Armageddon endpoint: <code>prices.runescape.wiki/api/v1/dmm</code>

===Latest price (all items)===
* <code>[https://prices.runescape.wiki/api/v1/osrs/latest '''/latest''']</code>

Get the latest high and low prices for the items that we have data for, and the [[wikipedia:Unix timestamp|Unix timestamp]] when that transaction took place.

Map from itemId (see [[Module:GEIDs/data.json|here]] for a reference) to an object of {high, highTime, low, lowTime}. If we've never seen an item traded, it won't be in the response. If we've never seen an instant-buy price, then <code>high</code> and <code>highTime</code> will be null (and similarly for <code>low</code> and <code>lowTime</code> if we've never seen an instant-sell).

====Query parameters====
* <code>id</code> - ''(optional)'' Item ID. If provided, will only display the latest price for this item. Example: [https://prices.runescape.wiki/api/v1/osrs/latest?id=4151 Abyssal whip]

{{Colour|Red|'''There's almost no scenario where you should be using the <code>id</code> parameter to loop over every item. If you need to get all 3700 item prices, don't hit us with 3700 API requests – just don't use the <code>id</code> parameter. It will result in about 100 times less resources used on both your side and ours.'''}}

<hr>
===Mapping ===
* <code>[https://prices.runescape.wiki/api/v1/osrs/mapping '''/mapping''']</code>
Gives a list of objects containing the name, id, examine text, members status, lowalch, highalch, GE buy limit, [[Value|value]], icon file name (on the wiki).
<pre>
[
    {"examine":"Fabulously ancient mage protection enchanted in the 3rd Age.","id":10344,"members":true,"lowalch":20200,"limit":8,"value":50500,"highalch":30300,"icon":"3rd age amulet.png","name":"3rd age amulet"},
    ...,
    {"examine":"A powerful staff.","id":22647,"members":true,"lowalch":120000,"limit":10,"value":300000,"highalch":180000,"icon":"Zuriel's staff.png","name":"Zuriel's staff"}
]
</pre>
<hr>

===5-minute prices===
* <code>[https://prices.runescape.wiki/api/v1/osrs/5m '''/5m''']</code>

Gives 5-minute average of item high and low prices as well as the number traded for the items that we have data on. Comes with a Unix timestamp indicating the 5 minute block the data is from.

====Query parameters====
* <code>timestamp</code> - ''(optional)'' Timestep to return prices for. If provided, will display 5-minute averages for all items we have data on for this time. The <code>timestamp</code> field represents the beginning of the 5-minute period being averaged. Example: [https://prices.runescape.wiki/api/v1/osrs/5m?timestamp=1615733400]

<hr>
===1-hour prices===
* <code>[https://prices.runescape.wiki/api/v1/osrs/1h '''/1h''']</code>

Gives hourly average of item high and low prices, and the number traded.

====Query parameters====
* <code>timestamp</code> - ''(optional)'' Timestep to return prices for. If provided, will display 1-hour averages for all items we have data on for this time. The <code>timestamp</code> field represents the beginning of the 1-hour period being averaged. Example: [https://prices.runescape.wiki/api/v1/osrs/1h?timestamp=1615734000]

<hr>
===Time-series===
* <code>[https://prices.runescape.wiki/api/v1/osrs/timeseries?timestep=5m&id=4151 '''/timeseries''']</code>

Gives a list of the high and low prices of item with the given id at the given interval, up to a maximum of 365 data points. Using a higher interval will return data going back further in time.

====Query parameters====
* <code>id</code> - ''(required)'' Item id to return a time-series for.
* <code>timestep</code> - ''(required)'' Timestep of the time-series. Valid options are "5m", "1h", "6h" and "24h".
Example: [https://prices.runescape.wiki/api/v1/osrs/timeseries?timestep=5m&id=4151]
