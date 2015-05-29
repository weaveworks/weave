import operator

import flask

from google.appengine.ext import ndb

app = flask.Flask('scheduler')
app.debug = True

# We use exponential moving average to record
# test run times.  Higher alpha discounts historic
# observations faster.
alpha = 0.3

class Test(ndb.Model):
  total_run_time = ndb.FloatProperty(default=0.) # Not total, but a EWMA
  total_runs = ndb.IntegerProperty(default=0)

class Schedule(ndb.Model):
  shards = ndb.JsonProperty()

@app.route('/record/<test_name>/<runtime>', methods=['POST'])
@ndb.transactional
def record(test_name, runtime):
  test = Test.get_by_id(test_name)
  if test is None:
    test = Test(id=test_name)
  test.total_run_time = (test.total_run_time * (1-alpha)) + (float(runtime) * alpha)
  test.total_runs += 1
  test.put()
  return ('', 204)

@app.route('/schedule/<int:test_run>/<int:shard_count>/<int:shard>', methods=['POST'])
def schedule(test_run, shard_count, shard):
  # read tests from body
  test_names = flask.request.get_json(force=True)['tests']

  # first see if we have a scedule already
  schedule_id = "%d-%d" % (test_run, shard_count)
  schedule = Schedule.get_by_id(schedule_id)
  if schedule is not None:
    return flask.json.jsonify(tests=schedule.shards[str(shard)])

  # if not, do simple greedy algorithm
  test_times = ndb.get_multi(ndb.Key(Test, test_name) for test_name in test_names)
  def avg(test):
    if test is not None:
      return test.total_run_time
    return 1
  test_times = [(test_name, avg(test)) for test_name, test in zip(test_names, test_times)]
  test_times_dict = dict(test_times)
  test_times.sort(key=operator.itemgetter(1))

  shards = {i: [] for i in xrange(shard_count)}
  while test_times:
    test_name, time = test_times.pop()

    # find shortest shard and put it in that
    s, _ = min(((i, sum(test_times_dict[t] for t in shards[i]))
      for i in xrange(shard_count)), key=operator.itemgetter(1))

    shards[s].append(test_name)

  # atomically insert or retrieve existing schedule
  schedule = Schedule.get_or_insert(schedule_id, shards=shards)
  return flask.json.jsonify(tests=schedule.shards[str(shard)])
